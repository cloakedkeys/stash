package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
	sqlite3 "github.com/mattn/go-sqlite3"

	"github.com/stashapp/stash/pkg/database"
	"github.com/stashapp/stash/pkg/sqlite"
)

const (
	sqliteMigrationsTable = "schema_migrations"
	customFieldsSuffix    = "_custom_fields"
	// how many problem rows to list before saying how many more there are
	maxReportedIssues = 20
)

// Tables that only hold data from before blobs were added. Stash hasn't read them since.
var legacySQLiteTables = []string{"movies_images", "performers_image", "scenes_cover", "studios_image", "tags_image"}

// Tables a new PostgreSQL database already has rows in: its first account, which the copy replaces.
var seededTables = []string{"users", "user_roles"}

// MigrateFromSQLite copies a SQLite library into an empty PostgreSQL database. The SQLite file is
// only read, never changed. The copy happens in one transaction, so if anything goes wrong the
// PostgreSQL database stays empty and it's safe to run again.
func MigrateFromSQLite(ctx context.Context, sqlitePath string, postgresPath string, out io.Writer) error {
	dsn, _ := strings.CutPrefix(postgresPath, string(database.PostgresBackend)+":")

	source, err := openSQLiteSource(sqlitePath)
	if err != nil {
		return err
	}
	defer source.Close()

	// one read transaction, so that every table comes from the same moment
	sourceTx, err := source.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("can't read this as a Stash SQLite database: %w", err)
	}
	defer func() { _ = sourceTx.Rollback() }()

	if err := checkSQLiteVersion(sourceTx); err != nil {
		return err
	}

	// creates the tables on a new database
	fmt.Fprintln(out, "Preparing the PostgreSQL database")
	target := NewDatabase()
	if err := target.Open(ctx, dsn); err != nil {
		return fmt.Errorf("opening PostgreSQL database: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("closing PostgreSQL database: %w", err)
	}

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to PostgreSQL database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	plans, err := planSQLiteCopy(ctx, sourceTx, conn, out)
	if err != nil {
		return err
	}
	if err := checkTargetEmpty(ctx, conn, plans); err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting PostgreSQL transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// rows go in table by table, so a row can arrive before one it refers to
	if _, err := tx.Exec(ctx, "SET LOCAL session_replication_role = replica"); err != nil {
		return fmt.Errorf("preparing to copy, which needs the same permission Stash needs to start: %w", err)
	}

	// the starter account goes, and the library's accounts replace it
	names := make([]string, len(plans))
	for i, plan := range plans {
		names[i] = quoteIdent(plan.table)
	}
	if _, err := tx.Exec(ctx, "TRUNCATE "+strings.Join(names, ", ")); err != nil {
		return fmt.Errorf("clearing the PostgreSQL database: %w", err)
	}

	var issues []string
	var totalRows int64
	for _, plan := range plans {
		n, tableIssues, err := plan.copy(ctx, sourceTx, tx)
		if err != nil {
			return fmt.Errorf("copying %s: %w", plan.table, err)
		}
		issues = append(issues, tableIssues...)
		totalRows += n
		if n > 0 {
			fmt.Fprintf(out, "Copied %s: %d rows\n", plan.table, n)
		}
	}

	if len(issues) > 0 {
		return issuesError(issues)
	}

	if err := resetSequences(ctx, sourceTx, tx, plans); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	fmt.Fprintln(out, "Updating PostgreSQL's statistics")
	if _, err := conn.Exec(ctx, "ANALYZE"); err != nil {
		return fmt.Errorf("analyzing: %w", err)
	}

	fmt.Fprintf(out, "Finished: copied %d rows from %d tables. The SQLite file wasn't changed.\n", totalRows, len(plans))
	return nil
}

func quoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

func issuesError(issues []string) error {
	shown := issues
	more := ""
	if len(shown) > maxReportedIssues {
		shown = shown[:maxReportedIssues]
		more = fmt.Sprintf("\n  and %d more", len(issues)-maxReportedIssues)
	}
	return fmt.Errorf("nothing was copied, because %d values can't be stored in PostgreSQL. Fix them in Stash and run this again:\n  %s%s",
		len(issues), strings.Join(shown, "\n  "), more)
}

func openSQLiteSource(path string) (*sqlx.DB, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading SQLite file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a folder, not a SQLite file", path)
	}

	escaped := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(filepath.ToSlash(path))
	db, err := sqlx.Open("sqlite3", "file:"+escaped+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening SQLite file: %w", err)
	}
	return db, nil
}

func checkSQLiteVersion(source *sqlx.Tx) error {
	var version uint
	var dirty bool
	err := source.QueryRow("SELECT version, dirty FROM "+sqliteMigrationsTable+" LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		return fmt.Errorf("can't read this as a Stash SQLite database: %w", err)
	}

	want := sqlite.NewDatabase().AppSchemaVersion()
	switch {
	case dirty:
		return fmt.Errorf("the SQLite database is at version %d, but an upgrade to it didn't finish. Restore it from a backup first", version)
	case version < want:
		return fmt.Errorf("the SQLite database is at version %d, and this build needs %d. Start this build of Stash on the SQLite database once to upgrade it, stop Stash, then run this again", version, want)
	case version > want:
		return fmt.Errorf("the SQLite database is at version %d, which is newer than this build's %d. Use a build of Stash at least as new as the one that last opened it", version, want)
	}
	return nil
}

type copyColumn struct {
	name      string
	dataType  string
	maxLen    int // characters, or 0 for no limit
	nullable  bool
	serial    bool
	inSource  bool
	asRawText bool // read as text, because the SQLite driver turns dates it can't read into year 1
}

type copyPlan struct {
	table   string
	columns []copyColumn
}

func planSQLiteCopy(ctx context.Context, source *sqlx.Tx, conn *pgx.Conn, out io.Writer) ([]copyPlan, error) {
	targetColumns, err := loadTargetColumns(ctx, conn)
	if err != nil {
		return nil, err
	}

	sourceColumns, err := loadSQLiteColumns(source)
	if err != nil {
		return nil, err
	}

	// a table only SQLite has is either old, or was added by something other than Stash
	for table := range sourceColumns {
		if _, ok := targetColumns[table]; ok || slices.Contains(legacySQLiteTables, table) {
			continue
		}
		var count int64
		if err := source.Get(&count, "SELECT COUNT(*) FROM "+quoteIdent(table)); err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		if count > 0 {
			fmt.Fprintf(out, "WARNING: not copying %s (%d rows). Stash doesn't use this table, so a plugin may have added it.\n", table, count)
		}
	}

	tables := make([]string, 0, len(targetColumns))
	for table := range targetColumns {
		tables = append(tables, table)
	}
	slices.Sort(tables)

	plans := make([]copyPlan, 0, len(tables))
	for _, table := range tables {
		inSource, ok := sourceColumns[table]
		if !ok {
			return nil, fmt.Errorf("the SQLite database has no %s table, so it doesn't match this build", table)
		}
		plan, err := planTable(table, targetColumns[table], inSource)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func planTable(table string, columns []copyColumn, inSource map[string]bool) (copyPlan, error) {
	for name := range inSource {
		if !slices.ContainsFunc(columns, func(c copyColumn) bool { return c.name == name }) {
			return copyPlan{}, fmt.Errorf("the SQLite column %s.%s has no match in PostgreSQL, so it would be lost", table, name)
		}
	}

	for i := range columns {
		c := &columns[i]
		c.inSource = inSource[c.name]
		c.asRawText = isTimeType(c.dataType)
		if !c.inSource && !isCustomFieldType(table, c.name) && !c.nullable {
			return copyPlan{}, fmt.Errorf("the PostgreSQL column %s.%s is required, but SQLite doesn't have it", table, c.name)
		}
		if !supportedTargetType(c.dataType) {
			return copyPlan{}, fmt.Errorf("the PostgreSQL column %s.%s has the type %s, which can't be copied into", table, c.name, c.dataType)
		}
	}
	return copyPlan{table: table, columns: columns}, nil
}

func isCustomFieldType(table, column string) bool {
	return column == "type" && strings.HasSuffix(table, customFieldsSuffix)
}

func loadTargetColumns(ctx context.Context, conn *pgx.Conn) (map[string][]copyColumn, error) {
	rows, err := conn.Query(ctx, `SELECT c.table_name, c.column_name, c.data_type, COALESCE(c.character_maximum_length, 0), c.is_nullable = 'YES',
		c.is_identity = 'YES' OR COALESCE(c.column_default, '') LIKE 'nextval(%'
		FROM information_schema.columns c
		JOIN pg_tables t ON t.schemaname = c.table_schema AND t.tablename = c.table_name
		WHERE c.table_schema = current_schema() AND c.table_name <> $1 AND c.is_generated = 'NEVER'
		ORDER BY c.table_name, c.ordinal_position`, sqliteMigrationsTable)
	if err != nil {
		return nil, fmt.Errorf("reading PostgreSQL tables: %w", err)
	}
	defer rows.Close()

	ret := make(map[string][]copyColumn)
	for rows.Next() {
		var table string
		var c copyColumn
		var maxLen int32
		if err := rows.Scan(&table, &c.name, &c.dataType, &maxLen, &c.nullable, &c.serial); err != nil {
			return nil, fmt.Errorf("reading PostgreSQL tables: %w", err)
		}
		c.maxLen = int(maxLen)
		ret[table] = append(ret[table], c)
	}
	return ret, rows.Err()
}

func loadSQLiteColumns(source *sqlx.Tx) (map[string]map[string]bool, error) {
	var tables []string
	if err := source.Select(&tables, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> ?", sqliteMigrationsTable); err != nil {
		return nil, fmt.Errorf("reading SQLite tables: %w", err)
	}

	ret := make(map[string]map[string]bool, len(tables))
	for _, table := range tables {
		var columns []string
		if err := source.Select(&columns, "SELECT name FROM pragma_table_info(?)", table); err != nil {
			return nil, fmt.Errorf("reading SQLite table %s: %w", table, err)
		}
		ret[table] = make(map[string]bool, len(columns))
		for _, column := range columns {
			ret[table][column] = true
		}
	}
	return ret, nil
}

func checkTargetEmpty(ctx context.Context, conn *pgx.Conn, plans []copyPlan) error {
	var used []string
	for _, plan := range plans {
		if slices.Contains(seededTables, plan.table) {
			continue
		}
		var any bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+quoteIdent(plan.table)+")").Scan(&any); err != nil {
			return fmt.Errorf("checking %s: %w", plan.table, err)
		}
		if any {
			used = append(used, plan.table)
		}
	}

	var users int
	if err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		return fmt.Errorf("checking users: %w", err)
	}
	if users > 1 {
		used = append(used, "users")
	}

	if len(used) > 0 {
		return fmt.Errorf("the PostgreSQL database already has data, in: %s. This only copies into an empty database, so that nothing there is overwritten", strings.Join(used, ", "))
	}
	return nil
}

// copy streams the table's rows, so even large images don't all sit in memory.
func (p copyPlan) copy(ctx context.Context, source *sqlx.Tx, target pgx.Tx) (int64, []string, error) {
	var selects []string
	var names []string
	for _, c := range p.columns {
		names = append(names, c.name)
		switch {
		case !c.inSource:
			continue
		case c.asRawText:
			selects = append(selects, "CAST("+quoteIdent(c.name)+" AS TEXT)")
		default:
			selects = append(selects, quoteIdent(c.name))
		}
	}

	rows, err := source.Query("SELECT " + strings.Join(selects, ", ") + " FROM " + quoteIdent(p.table))
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	src := &sqliteRowSource{plan: p, rows: rows, values: make([]any, len(selects))}
	src.pointers = make([]any, len(selects))
	for i := range src.values {
		src.pointers[i] = &src.values[i]
	}

	n, err := target.CopyFrom(ctx, pgx.Identifier{p.table}, names, src)
	if err != nil {
		return 0, nil, err
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	// rows with a problem were left out, and fail the whole copy
	if want := int64(src.read - len(src.issues)); n != want && len(src.issues) == 0 {
		return 0, nil, fmt.Errorf("read %d rows, but PostgreSQL received %d", want, n)
	}
	return n, src.issues, nil
}

// sqliteRowSource feeds rows to PostgreSQL's COPY. A row with a value PostgreSQL can't store is
// left out and noted, so that all the problems can be listed together.
type sqliteRowSource struct {
	plan     copyPlan
	rows     *sql.Rows
	values   []any
	pointers []any
	current  []any
	issues   []string
	read     int
	err      error
}

func (s *sqliteRowSource) Next() bool {
	for s.rows.Next() {
		if err := s.rows.Scan(s.pointers...); err != nil {
			s.err = err
			return false
		}
		s.read++
		var problems []string
		s.current, problems = s.plan.convertRow(s.values)
		if len(problems) == 0 {
			return true
		}
		s.issues = append(s.issues, problems...)
	}
	return false
}

func (s *sqliteRowSource) Values() ([]any, error) { return s.current, nil }

func (s *sqliteRowSource) Err() error { return s.err }

func (p copyPlan) convertRow(values []any) ([]any, []string) {
	ret := make([]any, len(p.columns))
	var problems []string
	customField := strings.HasSuffix(p.table, customFieldsSuffix)

	src := 0
	for i, c := range p.columns {
		if !c.inSource {
			continue
		}
		v := values[src]
		src++

		var err error
		if customField && c.name == "value" {
			var value any
			var goType string
			value, goType, err = convertCustomFieldValue(v)
			if err == nil {
				ret[i] = value
				if t := slices.IndexFunc(p.columns, func(c copyColumn) bool { return isCustomFieldType(p.table, c.name) }); t >= 0 {
					ret[t] = goType
				}
			}
		} else {
			ret[i], err = convertValue(v, c)
		}

		if err != nil {
			problems = append(problems, fmt.Sprintf("%s, row with %s = %v, column %s: %s", p.table, p.columns[0].name, values[0], c.name, err))
		}
	}
	return ret, problems
}

// convertCustomFieldValue returns a custom field as PostgreSQL holds it: as JSON, along with the
// name of its Go type, which is how it knows what to turn the JSON back into.
func convertCustomFieldValue(v any) (any, string, error) {
	var value any
	switch t := v.(type) {
	case []byte:
		value = string(t)
	case int64, float64, string:
		value = t
	case bool:
		// SQLite has no real booleans, and gives this back as a number
		if t {
			value = int64(1)
		} else {
			value = int64(0)
		}
	default:
		return nil, "", fmt.Errorf("unsupported custom field value of type %T", v)
	}

	if s, ok := value.(string); ok {
		if err := checkText(s, 0); err != nil {
			return nil, "", err
		}
	}
	if f, ok := value.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
		return nil, "", errors.New("not a number that can be stored")
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return string(encoded), fmt.Sprintf("%T", value), nil
}

func isTimeType(dataType string) bool {
	return dataType == "date" || dataType == "timestamp with time zone" || dataType == "timestamp without time zone"
}

func supportedTargetType(dataType string) bool {
	switch dataType {
	case "smallint", "integer", "bigint", "double precision", "boolean", "text", "character varying", "bytea", "jsonb":
		return true
	}
	return isTimeType(dataType)
}

func convertValue(v any, c copyColumn) (any, error) {
	if v == nil {
		return nil, nil
	}

	switch c.dataType {
	case "smallint":
		return convertInt(v, math.MinInt16, math.MaxInt16)
	case "integer":
		return convertInt(v, math.MinInt32, math.MaxInt32)
	case "bigint":
		return convertInt(v, math.MinInt64, math.MaxInt64)
	case "double precision":
		switch t := v.(type) {
		case float64:
			return t, nil
		case int64:
			return float64(t), nil
		}
	case "boolean":
		switch t := v.(type) {
		case bool:
			return t, nil
		case int64:
			return t != 0, nil
		}
	case "text", "character varying":
		return convertText(v, c.maxLen)
	case "bytea":
		switch t := v.(type) {
		case []byte:
			return t, nil
		case string:
			return []byte(t), nil
		}
	case "jsonb":
		return convertJSON(v, c)
	case "date", "timestamp with time zone", "timestamp without time zone":
		return convertTime(v, c)
	}
	return nil, fmt.Errorf("can't store a value of type %T as %s", v, c.dataType)
}

func convertInt(v any, lo, hi int64) (any, error) {
	var i int64
	switch t := v.(type) {
	case int64:
		i = t
	case bool:
		if t {
			i = 1
		}
	case float64:
		if t != math.Trunc(t) || t < float64(lo) || t > float64(hi) {
			return nil, fmt.Errorf("%v isn't a whole number in range", t)
		}
		i = int64(t)
	default:
		return nil, fmt.Errorf("%q isn't a whole number", fmt.Sprint(v))
	}
	if i < lo || i > hi {
		return nil, fmt.Errorf("%d is out of range", i)
	}
	return i, nil
}

func convertText(v any, maxLen int) (any, error) {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case []byte:
		s = string(t)
	case int64:
		// how SQLite holds phash fingerprints
		return strconv.FormatInt(t, 10), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(t), nil
	case time.Time:
		return t.Format(time.RFC3339Nano), nil
	default:
		return nil, fmt.Errorf("can't store a value of type %T as text", v)
	}
	if err := checkText(s, maxLen); err != nil {
		return nil, err
	}
	return s, nil
}

func checkText(s string, maxLen int) error {
	if !utf8.ValidString(s) {
		return errors.New("isn't valid UTF-8 text")
	}
	if strings.ContainsRune(s, 0) {
		return errors.New("contains a null character")
	}
	if maxLen > 0 && utf8.RuneCountInString(s) > maxLen {
		return fmt.Errorf("is longer than the %d characters allowed", maxLen)
	}
	return nil
}

func convertJSON(v any, c copyColumn) (any, error) {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case []byte:
		s = string(t)
	default:
		return nil, fmt.Errorf("can't store a value of type %T as JSON", v)
	}
	if strings.TrimSpace(s) == "" && c.nullable {
		return nil, nil
	}
	if !json.Valid([]byte(s)) {
		return nil, errors.New("isn't valid JSON")
	}
	if strings.Contains(s, `\u0000`) {
		return nil, errors.New("contains a null character")
	}
	return s, nil
}

// convertTime parses a date or timestamp, using the same formats as the SQLite driver. Unlike the
// driver, it reports what it can't read, instead of turning it into year 1.
func convertTime(v any, c copyColumn) (any, error) {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case []byte:
		s = string(t)
	default:
		return nil, fmt.Errorf("can't read a value of type %T as a date", v)
	}

	if strings.TrimSpace(s) == "" && c.nullable {
		return nil, nil
	}

	trimmed := strings.TrimSuffix(s, "Z")
	for _, format := range sqlite3.SQLiteTimestampFormats {
		if t, err := time.ParseInLocation(format, trimmed, time.UTC); err == nil {
			if c.dataType == "timestamp without time zone" {
				return t.UTC(), nil
			}
			return t, nil
		}
	}
	return nil, fmt.Errorf("%q isn't a date PostgreSQL can store", s)
}

// resetSequences carries on the numbering from where SQLite was, so new rows get new IDs, and IDs
// of deleted rows aren't used again.
func resetSequences(ctx context.Context, source *sqlx.Tx, target pgx.Tx, plans []copyPlan) error {
	used := make(map[string]int64)
	var hasSequences bool
	if err := source.Get(&hasSequences, "SELECT COUNT(*) > 0 FROM sqlite_master WHERE name = 'sqlite_sequence'"); err != nil {
		return fmt.Errorf("reading SQLite sequences: %w", err)
	}
	if hasSequences {
		rows, err := source.Query("SELECT name, seq FROM sqlite_sequence")
		if err != nil {
			return fmt.Errorf("reading SQLite sequences: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var seq int64
			if err := rows.Scan(&name, &seq); err != nil {
				return fmt.Errorf("reading SQLite sequences: %w", err)
			}
			used[name] = seq
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("reading SQLite sequences: %w", err)
		}
	}

	for _, plan := range plans {
		for _, c := range plan.columns {
			if !c.serial {
				continue
			}
			q := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence($1, $2), GREATEST(COALESCE(MAX(%[1]s), 0), $3::bigint, 1), GREATEST(COALESCE(MAX(%[1]s), 0), $3::bigint) > 0) FROM %[2]s`, quoteIdent(c.name), quoteIdent(plan.table))
			var ignored int64
			if err := target.QueryRow(ctx, q, quoteIdent(plan.table), c.name, used[plan.table]).Scan(&ignored); err != nil {
				return fmt.Errorf("setting numbering for %s.%s: %w", plan.table, c.name, err)
			}
		}
	}
	return nil
}
