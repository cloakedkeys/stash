package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
)

type queryPagination struct {
	page    int
	perPage int
}

type queryBuilder struct {
	repository *repository

	columns []string
	from    string

	joins         joins
	whereClauses  []string
	havingClauses []string
	withClauses   []string
	recursiveWith bool

	withArgs   []interface{}
	joinArgs   []interface{}
	whereArgs  []interface{}
	havingArgs []interface{}

	sort string
	// idColumn is the item id the query returns, set by distinctIDs. It is the
	// last sort key, so the order is fixed between pages.
	idColumn       string
	pagination     *queryPagination
	groupByClauses []string
}

func (qb queryBuilder) allArgs() []interface{} {
	var args []interface{}
	args = append(args, qb.withArgs...)
	args = append(args, qb.joinArgs...)
	args = append(args, qb.whereArgs...)
	args = append(args, qb.havingArgs...)
	return args
}

func (qb queryBuilder) body(includeSortPagination bool) string {
	return fmt.Sprintf("SELECT %s FROM %s%s", strings.Join(qb.columns, ", "), qb.from, qb.joins.toSQL(includeSortPagination))
}

func (qb *queryBuilder) addColumn(column string) {
	qb.columns = append(qb.columns, column)
}

func (qb *queryBuilder) addGroupBy(columns ...string) {
	if len(columns) > 0 {
		qb.groupByClauses = sliceutil.AppendUniques(qb.groupByClauses, columns)
	}
}

// groupBy returns the group by columns for the query. Sort-only joins are left
// out when not sorting, so columns from those tables must be left out as well.
func (qb queryBuilder) groupBy(includeSortPagination bool) []string {
	if includeSortPagination {
		return qb.groupByClauses
	}

	sortOnly := make(map[string]bool)
	for _, j := range qb.joins {
		if j.sort {
			sortOnly[j.alias()] = true
		}
	}

	var ret []string
	for _, column := range qb.groupByClauses {
		table, _, found := strings.Cut(column, ".")
		if !found || !sortOnly[table] {
			ret = append(ret, column)
		}
	}
	return ret
}

func (qb queryBuilder) toSQL(includeSortPagination bool) string {
	body := qb.body(includeSortPagination)

	withClause := ""
	if len(qb.withClauses) > 0 {
		var recursive string
		if qb.recursiveWith {
			recursive = " RECURSIVE "
		}
		withClause = "WITH " + recursive + strings.Join(qb.withClauses, ", ") + " "
	}

	body = withClause + qb.repository.buildQueryBody(body, qb.whereClauses, qb.havingClauses, qb.groupBy(includeSortPagination))
	if includeSortPagination {
		body += qb.sort
		if qb.pagination != nil {
			body += getPaginationSQL(qb.pagination)
		}
	}

	return body
}

// withSortOrder returns a copy of the query with a sort_order column, the
// row's position in the query's sort, and the case-folding tables if the sort
// needs them.
func (qb queryBuilder) withSortOrder() queryBuilder {
	sort := qb.stableSort()
	if strings.Contains(sort, naturalFoldCTE) {
		with, args := naturalFoldWith()
		qb.withClauses = append([]string{with}, qb.withClauses...)
		qb.withArgs = append(args, qb.withArgs...)
	}
	qb.columns = append(append([]string{}, qb.columns...), "row_number() OVER ("+sort+") AS sort_order")
	return qb
}

// naturalCollation is the collation of name and title sorts.
const naturalCollation = `COLLATE "natural_ci"`

// stableSort returns the query's sort, in SQLite's order, with the item id as
// the last key, so that items the sort ranks equal, or all items when there is
// no sort, come back in the same order on every page.
//
// With no sort, PostgreSQL uses id order, for a stable result. (SQLite's order
// there depends on which index it reads, so it isn't matched.)
//
// With a sort, each key sorts as on SQLite:
//   - Empty values (NULL) come first in ascending order and last in
//     descending order.
//   - Names and titles sort by naturalSortKey, which sorts as SQLite's
//     NATURAL_CI collation does: names that differ only in case are equal,
//     and characters sort by their code.
//   - SQLite's NATURAL_CI collation never finds two names equal, so once a
//     sort reaches a name key that has a value, that key decides, and the keys
//     after it are never used. Those keys are only used here when every name
//     key before them is empty (NULL), since SQLite doesn't pass empty values
//     to the collation and moves on to the next key.
//
// Then the id runs the way SQLite gives tied items. The first name key that
// has a value decides: same names come back newest first when that key is
// ascending, and oldest first when it is descending. Items with no name key
// that has a value come back in id order.
func (qb queryBuilder) stableSort() string {
	if qb.idColumn == "" {
		return qb.sort
	}
	if strings.TrimSpace(qb.sort) == "" {
		return " ORDER BY " + qb.idColumn + " ASC"
	}

	keys := parseSortKeys(qb.sort)
	var parts []string
	var names []string
	for _, k := range keys {
		parts = append(parts, k.sql(names))
		if k.natural {
			names = append(names, k.expr)
		}
	}
	return " ORDER BY " + strings.Join(parts, ", ") + ", " + tieKey(keys, qb.idColumn)
}

// sortKey is one key of an ORDER BY clause.
type sortKey struct {
	expr string
	// natural is whether the key sorts with the natural_ci collation.
	natural bool
	desc    bool
}

// parseSortKeys splits an ORDER BY clause into its keys. Any NULLS FIRST or
// NULLS LAST is dropped, since sortKey.sql puts empty values where SQLite
// does.
func parseSortKeys(sort string) []sortKey {
	var ret []sortKey
	for _, key := range sortKeys(sort) {
		k := sortKey{expr: strings.TrimSpace(key)}
		for _, nulls := range []string{"NULLS FIRST", "NULLS LAST"} {
			if s, ok := cutSuffixFold(k.expr, nulls); ok {
				k.expr = s
				break
			}
		}
		if s, ok := cutSuffixFold(k.expr, "DESC"); ok {
			k.expr, k.desc = s, true
		} else if s, ok := cutSuffixFold(k.expr, "ASC"); ok {
			k.expr = s
		}
		if s, ok := cutSuffixFold(k.expr, naturalCollation); ok {
			k.expr, k.natural = s, true
		}
		ret = append(ret, k)
	}
	return ret
}

// cutSuffixFold returns s without the word or words suffix at its end, ignoring
// case, and whether it was there.
func cutSuffixFold(s, suffix string) (string, bool) {
	if len(s) <= len(suffix) || !strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s, false
	}
	before := s[:len(s)-len(suffix)]
	trimmed := strings.TrimRight(before, " \t\n")
	if len(trimmed) == len(before) {
		// part of a longer word, such as a column ending in "asc"
		return s, false
	}
	return trimmed, true
}

// sql returns the key as SQLite sorts it, where names are the name keys
// before it.
func (k sortKey) sql(names []string) string {
	expr := k.expr
	if k.natural {
		expr = naturalSortKey(expr)
	}
	if len(names) > 0 {
		var empty []string
		for _, n := range names {
			empty = append(empty, "("+n+") IS NULL")
		}
		expr = "CASE WHEN " + strings.Join(empty, " AND ") + " THEN " + expr + " END"
		if k.natural {
			expr += ` COLLATE "C"`
		}
	}
	if k.desc {
		return expr + " DESC NULLS LAST"
	}
	return expr + " ASC NULLS FIRST"
}

// tieKey returns the last sort key, the id, for items that sort ranks equal.
// The id runs opposite to the first name key that isn't NULL, or ascending if
// there is none.
func tieKey(keys []sortKey, idColumn string) string {
	var whens []string
	for _, k := range keys {
		if !k.natural {
			continue
		}
		id := "-" + idColumn
		if k.desc {
			id = idColumn
		}
		whens = append(whens, " WHEN ("+k.expr+") IS NOT NULL THEN "+id)
	}
	if len(whens) == 0 {
		return idColumn + " ASC"
	}
	return "CASE" + strings.Join(whens, "") + " ELSE " + idColumn + " END ASC"
}

// sortKeys splits an ORDER BY clause into its keys, at the commas that aren't
// inside brackets or quotes.
func sortKeys(sort string) []string {
	sort = strings.TrimSpace(sort)
	if len(sort) >= len("ORDER BY") && strings.EqualFold(sort[:len("ORDER BY")], "ORDER BY") {
		sort = sort[len("ORDER BY"):]
	}

	var keys []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(sort); i++ {
		c := sort[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			keys = append(keys, sort[start:i])
			start = i + 1
		}
	}
	return append(keys, sort[start:])
}

// findIDs returns the IDs the query finds, sorted and paged. A sort or filter on
// files gives one row per file, so each item is kept once, at its first position.
func (qb queryBuilder) findIDs(ctx context.Context) ([]int, error) {
	const includeSortPagination = true
	sorted := qb.withSortOrder()
	sorted.sort = ""
	sorted.pagination = nil

	sql := "SELECT sorted.id FROM (" + sorted.toSQL(includeSortPagination) + ") AS sorted GROUP BY sorted.id ORDER BY MIN(sorted.sort_order)"
	if qb.pagination != nil {
		sql += getPaginationSQL(qb.pagination)
	}
	return qb.repository.runIdsQuery(ctx, sql, sorted.allArgs())
}

func (qb queryBuilder) executeFind(ctx context.Context) ([]int, int, error) {
	const includeSortPagination = true

	// Redirect
	if qb.pagination != nil && qb.pagination.perPage == 0 {
		res, err := qb.executeCount(ctx)
		return []int{}, res, err
	}

	sorted := qb.withSortOrder()
	body := sorted.body(includeSortPagination)
	pagination := getPaginationSQL(qb.pagination)
	return qb.repository.executeFindQuery(ctx, body, sorted.allArgs(), pagination, sorted.whereClauses, sorted.havingClauses, sorted.withClauses, sorted.groupByClauses, sorted.recursiveWith)
}

func (qb queryBuilder) executeCount(ctx context.Context) (int, error) {
	const includeSortPagination = false
	body := qb.body(includeSortPagination)

	withClause := ""
	if len(qb.withClauses) > 0 {
		var recursive string
		if qb.recursiveWith {
			recursive = " RECURSIVE "
		}
		withClause = "WITH " + recursive + strings.Join(qb.withClauses, ", ") + " "
	}

	body = qb.repository.buildQueryBody(body, qb.whereClauses, qb.havingClauses, qb.groupBy(includeSortPagination))
	countQuery := withClause + "SELECT COUNT(DISTINCT temp.id) as count FROM (" + body + ") as temp"
	return qb.repository.runCountQuery(ctx, countQuery, qb.allArgs())
}

func (qb *queryBuilder) addWhere(clauses ...string) {
	for _, clause := range clauses {
		if len(clause) > 0 {
			qb.whereClauses = append(qb.whereClauses, clause)
		}
	}
}

func (qb *queryBuilder) addHaving(clauses ...string) {
	for _, clause := range clauses {
		if len(clause) > 0 {
			qb.havingClauses = append(qb.havingClauses, clause)
		}
	}
}

func (qb *queryBuilder) addWith(recursive bool, clauses ...string) {
	for _, clause := range clauses {
		if len(clause) > 0 {
			qb.withClauses = append(qb.withClauses, clause)
		}
	}

	qb.recursiveWith = qb.recursiveWith || recursive
}

func (qb *queryBuilder) addArg(args ...interface{}) {
	qb.whereArgs = append(qb.whereArgs, args...)
}

func (qb *queryBuilder) addHavingArg(args ...interface{}) {
	qb.havingArgs = append(qb.havingArgs, args...)
}

func (qb *queryBuilder) hasJoin(alias string) bool {
	for _, j := range qb.joins {
		if j.alias() == alias {
			return true
		}
	}

	return false
}

func (qb *queryBuilder) join(table, as, onClause string) {
	newJoin := join{
		table:    table,
		as:       as,
		onClause: onClause,
		joinType: "LEFT",
	}

	qb.joins.add(newJoin)
}

func (qb *queryBuilder) joinSort(table, as, onClause string) {
	newJoin := join{
		sort:     false, // BUG: If we use this, we need to remake getSort since we do groupby on the args.
		table:    table,
		as:       as,
		onClause: onClause,
		joinType: "LEFT",
	}

	qb.joins.add(newJoin)
}

func (qb *queryBuilder) addJoins(joins ...join) {
	for _, j := range joins {
		if qb.joins.addUnique(j) {
			qb.joinArgs = append(qb.joinArgs, j.args...)
		}
	}
}

func (qb *queryBuilder) addFilter(f *filterBuilder) error {
	err := f.getError()
	if err != nil {
		return err
	}

	clause, args := f.generateWithClauses()
	if len(clause) > 0 {
		qb.addWith(f.recursiveWith, clause)
	}
	if len(args) > 0 {
		qb.withArgs = append(qb.withArgs, args...)
	}

	qb.addJoins(f.getAllJoins()...)

	clause, args = f.generateWhereClauses()
	if len(clause) > 0 {
		qb.addWhere(clause)
	}
	if len(args) > 0 {
		qb.addArg(args...)
	}

	clause, args = f.generateHavingClauses()
	if len(clause) > 0 {
		qb.addHaving(clause)
	}
	if len(args) > 0 {
		qb.addHavingArg(args...)
	}

	return nil
}

func (qb *queryBuilder) parseQueryString(columns []string, q string) {
	specs := models.ParseSearchString(q)

	// helper to wrap column with coalesce if it doesn't already have it
	wrapColumn := func(column string) string {
		// if column already has COALESCE or CAST, don't wrap again
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(column)), "COALESCE") ||
			strings.HasPrefix(strings.ToUpper(strings.TrimSpace(column)), "CAST") {
			return column
		}
		return coalesce(column)
	}

	// likeClause returns the LIKE predicate for a single column/term pair
	// and records the bound argument.
	// The built-in LIKE is already case-insensitive for ASCII, so pure-ASCII
	// terms keep it and pay no per-row cost; only terms containing non-ASCII
	// characters go through lower_unicode(), which calls back into Go for
	// every scanned row.
	likeClause := func(column, term, op string) string {
		qb.addArg(like(term))
		return wrapColumn(column) + " " + strings.ReplaceAll(op, "LIKE", "ILIKE") + " ? ESCAPE '\\'"
	}

	for _, t := range specs.MustHave {
		var clauses []string

		for _, column := range columns {
			clauses = append(clauses, likeClause(column, t, "LIKE"))
		}

		qb.addWhere("(" + strings.Join(clauses, " OR ") + ")")
	}

	for _, t := range specs.MustNot {
		for _, column := range columns {
			qb.addWhere(likeClause(column, t, "NOT LIKE"))
		}
	}

	for _, set := range specs.AnySets {
		var clauses []string

		for _, column := range columns {
			for _, v := range set {
				clauses = append(clauses, likeClause(column, v, "LIKE"))
			}
		}

		qb.addWhere("(" + strings.Join(clauses, " OR ") + ")")
	}
}
