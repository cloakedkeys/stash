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

	sort           string
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
// row's position in the query's sort.
func (qb queryBuilder) withSortOrder() queryBuilder {
	qb.columns = append(append([]string{}, qb.columns...), "row_number() OVER ("+qb.sort+") AS sort_order")
	return qb
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
	return qb.repository.runIdsQuery(ctx, sql, qb.allArgs())
}

func (qb queryBuilder) executeFind(ctx context.Context) ([]int, int, error) {
	const includeSortPagination = true
	body := qb.withSortOrder().body(includeSortPagination)

	// Redirect
	if qb.pagination != nil && qb.pagination.perPage == 0 {
		res, err := qb.executeCount(ctx)
		return []int{}, res, err
	}

	pagination := getPaginationSQL(qb.pagination)
	return qb.repository.executeFindQuery(ctx, body, qb.allArgs(), pagination, qb.whereClauses, qb.havingClauses, qb.withClauses, qb.groupByClauses, qb.recursiveWith)
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
		return wrapColumn(column) + " " + strings.ReplaceAll(op, "LIKE", "ILIKE") + " ?"
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
