package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stashapp/stash/pkg/models"
)

func illegalFilterCombination(type1, type2 string) error {
	return fmt.Errorf("cannot have %s and %s in the same filter", type1, type2)
}

func validateFilterCombination[T any](sf models.OperatorFilter[T]) error {
	const and = "AND"
	const or = "OR"
	const not = "NOT"

	if sf.And != nil {
		if sf.Or != nil {
			return illegalFilterCombination(and, or)
		}
		if sf.Not != nil {
			return illegalFilterCombination(and, not)
		}
	}

	if sf.Or != nil {
		if sf.Not != nil {
			return illegalFilterCombination(or, not)
		}
	}

	return nil
}

func handleSubFilter[T any](ctx context.Context, handler criterionHandler, f *filterBuilder, subFilter models.OperatorFilter[T]) {
	subQuery := &filterBuilder{}
	handler.handle(ctx, subQuery)

	if subFilter.And != nil {
		f.and(subQuery)
	}
	if subFilter.Or != nil {
		f.or(subQuery)
	}
	if subFilter.Not != nil {
		f.not(subQuery)
	}
}

// hasAllOf reports whether the filter or one of its sub-filters has ALL_OF
// filters.
func (f *filterBuilder) hasAllOf() bool {
	for ff := f; ff != nil; ff = ff.subFilter {
		if len(ff.allOf) > 0 {
			return true
		}
	}
	return false
}

// handleAllOf builds each of the ALL_OF filters of a level on its own.
func handleAllOf[T any](ctx context.Context, f *filterBuilder, filters []*T, newHandler func(*T) criterionHandler) {
	for _, sub := range filters {
		if sub != nil {
			f.allOf = append(f.allOf, filterBuilderFromHandler(ctx, newHandler(sub)))
		}
	}
}

type sqlClause struct {
	sql  string
	args []interface{}
}

func (c sqlClause) not() sqlClause {
	return sqlClause{
		sql:  "NOT (" + c.sql + ")",
		args: c.args,
	}
}

func makeClause(sql string, args ...interface{}) sqlClause {
	return sqlClause{
		sql:  sql,
		args: args,
	}
}

func joinClauses(joinType string, clauses ...sqlClause) sqlClause {
	var ret []string
	var args []interface{}

	for _, clause := range clauses {
		ret = append(ret, "("+clause.sql+")")
		args = append(args, clause.args...)
	}

	return sqlClause{sql: strings.Join(ret, " "+joinType+" "), args: args}
}

func orClauses(clauses ...sqlClause) sqlClause {
	return joinClauses("OR", clauses...)
}

func andClauses(clauses ...sqlClause) sqlClause {
	return joinClauses("AND", clauses...)
}

type joinType string

const (
	joinTypeLeft  joinType = "LEFT"
	joinTypeInner joinType = "INNER"
)

type join struct {
	table    string
	as       string
	onClause string
	joinType joinType
	args     []interface{}

	// if true, indicates this is required for sorting only
	sort bool

	// if true, this was an inner join that was made a left join because
	// its filter is combined with OR (#6914)
	leftForOr bool
}

// equals returns true if the other join alias/table is equal to this one
func (j join) equals(o join) bool {
	return j.alias() == o.alias()
}

// alias returns the as string, or the table if as is empty
func (j join) alias() string {
	if j.as == "" {
		return j.table
	}

	return j.as
}

func (j join) getJoinType() joinType {
	if j.joinType == "" {
		return joinTypeLeft
	}
	return j.joinType
}

func (j join) toSQL() string {
	asStr := ""
	joinStr := j.getJoinType()
	if j.as != "" && j.as != j.table {
		asStr = " AS " + j.as
	}

	return fmt.Sprintf("%s JOIN %s%s ON %s", joinStr, j.table, asStr, j.onClause)
}

type joins []join

// addUnique only adds if not already present
// returns true if added
func (j *joins) addUnique(newJoin join) bool {
	found := false
	for i, jj := range *j {
		if jj.equals(newJoin) {
			found = true
			// if sort is false on the new join, but true on the existing, set the false
			if !newJoin.sort && jj.sort {
				(*j)[i].sort = false
			}

			// if the new join is inner, override existing left join
			if newJoin.getJoinType() == joinTypeInner && jj.getJoinType() == joinTypeLeft {
				(*j)[i].joinType = joinTypeInner
			}

			break
		}
	}

	if !found {
		*j = append(*j, newJoin)
	}
	return !found
}

func (j *joins) add(newJoins ...join) {
	// only add if not already joined
	for _, newJoin := range newJoins {
		j.addUnique(newJoin)
	}
}

func (j *joins) toSQL(includeSortPagination bool) string {
	if len(*j) == 0 {
		return ""
	}

	var ret []string
	for _, jj := range *j {
		// skip sort-only joins if not including sort/pagination
		if !includeSortPagination && jj.sort {
			continue
		}
		ret = append(ret, jj.toSQL())
	}

	return " " + strings.Join(ret, " ")
}

type filterBuilder struct {
	subFilter   *filterBuilder
	subFilterOp string

	// filters that must all match as well (ALL_OF). They are made into
	// WHERE clauses of this level when the query is built.
	allOf []*filterBuilder

	joins         joins
	whereClauses  []sqlClause
	havingClauses []sqlClause
	withClauses   []sqlClause
	recursiveWith bool

	err error
}

func (f *filterBuilder) empty() bool {
	return f == nil || (len(f.whereClauses) == 0 && len(f.joins) == 0 && len(f.havingClauses) == 0 && f.subFilter == nil && len(f.allOf) == 0)
}

func filterBuilderFromHandler(ctx context.Context, handler criterionHandler) *filterBuilder {
	f := &filterBuilder{}
	handler.handle(ctx, f)
	return f
}

var errSubFilterAlreadySet = errors.New(`sub-filter already set`)

// sub-filter operator values
var (
	andOp = "AND"
	orOp  = "OR"
	notOp = "AND NOT"
)

// and sets the sub-filter that will be ANDed with this one.
// Sets the error state if sub-filter is already set.
func (f *filterBuilder) and(a *filterBuilder) {
	if f.subFilter != nil {
		f.setError(errSubFilterAlreadySet)
		return
	}

	f.subFilter = a
	f.subFilterOp = andOp
}

// or sets the sub-filter that will be ORed with this one.
// Sets the error state if a sub-filter is already set.
func (f *filterBuilder) or(o *filterBuilder) {
	if f.subFilter != nil {
		f.setError(errSubFilterAlreadySet)
		return
	}

	f.subFilter = o
	f.subFilterOp = orOp

	// #6914 - joined tables are inner joins by default, but if this is an OR filter,
	// they need to be left joins
	f.innerJoinsToLeftJoins()
	o.innerJoinsToLeftJoins()
}

// not sets the sub-filter that will be AND NOTed with this one.
// Sets the error state if a sub-filter is already set.
func (f *filterBuilder) not(n *filterBuilder) {
	if f.subFilter != nil {
		f.setError(errSubFilterAlreadySet)
		return
	}

	f.subFilter = n
	f.subFilterOp = notOp
}

// addJoin adds a join to the filter. The join is expressed in SQL as:
// <joinType> JOIN <table> [AS <as>] ON <onClause>
// The AS is omitted if as is empty.
// This method does not add a join if it its alias/table name is already
// present in another existing join.
func (f *filterBuilder) addJoin(joinType joinType, table, as, onClause string, args ...interface{}) {
	newJoin := join{
		table:    table,
		as:       as,
		onClause: onClause,
		joinType: joinType,
		args:     args,
	}

	f.joins.add(newJoin)
}

// addLeftJoin adds a left join to the filter. The join is expressed in SQL as:
// LEFT JOIN <table> [AS <as>] ON <onClause>
// The AS is omitted if as is empty.
// This method does not add a join if it its alias/table name is already
// present in another existing join.
func (f *filterBuilder) addLeftJoin(table, as, onClause string, args ...interface{}) {
	newJoin := join{
		table:    table,
		as:       as,
		onClause: onClause,
		joinType: joinTypeLeft,
		args:     args,
	}

	f.joins.add(newJoin)
}

// addInnerJoin adds an inner join to the filter. The join is expressed in SQL as:
// INNER JOIN <table> [AS <as>] ON <onClause>
// The AS is omitted if as is empty.
// This method does not add a join if it its alias/table name is already
// present in another existing join.
func (f *filterBuilder) addInnerJoin(table, as, onClause string, args ...interface{}) {
	newJoin := join{
		table:    table,
		as:       as,
		onClause: onClause,
		joinType: joinTypeInner,
		args:     args,
	}

	f.joins.add(newJoin)
}

func (f *filterBuilder) innerJoinsToLeftJoins() {
	for i := range f.joins {
		if f.joins[i].getJoinType() == joinTypeInner {
			f.joins[i].joinType = joinTypeLeft
			f.joins[i].leftForOr = true
		}
	}
}

// addWhere adds a where clause and arguments to the filter. Where clauses
// are ANDed together. Does not add anything if the provided string is empty.
func (f *filterBuilder) addWhere(sql string, args ...interface{}) {
	if sql == "" {
		return
	}
	f.whereClauses = append(f.whereClauses, makeClause(sql, args...))
}

// addHaving adds a where clause and arguments to the filter. Having clauses
// are ANDed together. Does not add anything if the provided string is empty.
func (f *filterBuilder) addHaving(sql string, args ...interface{}) {
	if sql == "" {
		return
	}
	f.havingClauses = append(f.havingClauses, makeClause(sql, args...))
}

// addWith adds a with clause and arguments to the filter
func (f *filterBuilder) addWith(sql string, args ...interface{}) {
	if sql == "" {
		return
	}

	f.withClauses = append(f.withClauses, makeClause(sql, args...))
}

// addRecursiveWith adds a with clause and arguments to the filter, and sets it to recursive
//
//nolint:unused
func (f *filterBuilder) addRecursiveWith(sql string, args ...interface{}) {
	if sql == "" {
		return
	}

	f.addWith(sql, args...)
	f.recursiveWith = true
}

func (f *filterBuilder) getSubFilterClause(clause, subFilterClause string) string {
	ret := clause

	if subFilterClause != "" {
		var op string
		if len(ret) > 0 {
			op = " " + f.subFilterOp + " "
		} else if f.subFilterOp == notOp {
			op = "NOT "
		}

		ret += op + "(" + subFilterClause + ")"
	}

	return ret
}

// hasOrNot reports whether the filter or one of its sub-filters is combined
// with OR or NOT.
func (f *filterBuilder) hasOrNot() bool {
	for ff := f; ff != nil; ff = ff.subFilter {
		if ff.subFilter != nil && ff.subFilterOp != andOp {
			return true
		}
	}
	return false
}

// sharesJoinName reports whether one of the joins has the alias of one of
// the other joins.
func (j joins) sharesJoinName(other joins) bool {
	for _, a := range j {
		for _, b := range other {
			if a.equals(b) {
				return true
			}
		}
	}
	return false
}

// isolateSubFilterLevels makes the conditions of some levels of the filter
// into one WHERE clause that selects the matching ids by itself:
//
//	<table>.id IN ([WITH <with>] SELECT <table>.id FROM <table> <joins>
//	  WHERE <where> [GROUP BY <table>.id HAVING <having>])
//
// Otherwise every level is checked row by row in one joined query:
//   - the HAVING clauses of the levels are always ANDed (#2167);
//   - a NOT on a joined condition matches an item when any one of its joined
//     rows fails the condition;
//   - an inner join that only filters is made a left join for OR (#6914), so
//     it stops filtering. Inside its own subquery a level keeps its inner
//     joins;
//   - two levels that join a table under the same name share one join, so
//     only one level's join is used, or both conditions must hold for the
//     same joined row (#2172);
//   - only the top level's WITH clauses go into the query, so a sub-filter
//     condition that needs one failed (#2170).
//
// A sub-filter level with WITH clauses is always isolated, with its WITH
// clauses inside its own subquery. There a level can also use the same WITH
// name as another level.
//
// When some level is combined with OR or NOT, every level that has joins or
// HAVING clauses, or that is under a NOT, is isolated. Each level then
// matches exactly the items it matches on its own, and AND, OR and NOT
// combine those sets. A level under NOT is isolated even without joins:
// NOT on a condition that is unknown for a NULL column would leave the item
// out of both the condition and its NOT.
//
// When all levels are ANDed, only a level that joins a table under a name
// that an earlier level also uses is isolated. Other filters are left as
// they are.
func (f *filterBuilder) isolateSubFilterLevels(table string) {
	orNot := f.hasOrNot()
	underNot := false
	// the joins that stay in the outer query
	var outerJoins joins
	for ff := f; ff != nil; ff = ff.subFilter {
		var isolate bool
		switch {
		case ff != f && len(ff.withClauses) > 0:
			isolate = true
		case orNot:
			isolate = len(ff.joins) > 0 || len(ff.havingClauses) > 0 || (underNot && len(ff.whereClauses) > 0)
		default:
			isolate = ff.joins.sharesJoinName(outerJoins)
		}
		underNot = underNot || ff.subFilterOp == notOp
		if !isolate {
			outerJoins = append(outerJoins, ff.joins...)
			continue
		}

		ff.isolate(table, ff != f)
	}
}

// isolate makes the conditions of this level, without its sub-filter, into
// one WHERE clause that selects the matching ids by itself. With withInside,
// the level's WITH clauses go into its subquery.
func (f *filterBuilder) isolate(table string, withInside bool) {
	var args []interface{}
	sql := table + ".id IN ("
	if withInside && len(f.withClauses) > 0 {
		with, a := f.generateWithClauses()
		sql += "WITH "
		if f.recursiveWith {
			sql += "RECURSIVE "
		}
		sql += with + " "
		args = append(args, a...)
		f.withClauses = nil
		f.recursiveWith = false
	}

	levelJoins := make(joins, len(f.joins))
	for i, j := range f.joins {
		if j.leftForOr {
			j.joinType = joinTypeInner
		}
		levelJoins[i] = j
		if !j.sort {
			args = append(args, j.args...)
		}
	}

	sql += fmt.Sprintf("SELECT %[1]s.id FROM %[1]s%[2]s", table, levelJoins.toSQL(false))
	if where, a := f.andClauses(f.whereClauses); where != "" {
		sql += " WHERE " + where
		args = append(args, a...)
	}
	if len(f.havingClauses) > 0 {
		having, a := f.andClauses(f.havingClauses)
		sql += fmt.Sprintf(" GROUP BY %s.id HAVING %s", table, having)
		args = append(args, a...)
	}
	sql += ")"

	f.whereClauses = []sqlClause{makeClause(sql, args...)}
	f.havingClauses = nil
	f.joins = nil
}

// generateWhereClauses generates the SQL where clause for this filter.
// All where clauses within the filter are ANDed together. This is combined
// with the sub-filter, which will use the applicable operator (AND/OR/AND NOT).
func (f *filterBuilder) generateWhereClauses() (clause string, args []interface{}) {
	clause, args = f.andClauses(f.whereClauses)

	if f.subFilter != nil {
		c, a := f.subFilter.generateWhereClauses()
		if c != "" {
			clause = f.getSubFilterClause(clause, c)
			if len(a) > 0 {
				args = append(args, a...)
			}
		}
	}

	return
}

// generateHavingClauses generates the SQL having clause for this filter.
// All having clauses within the filter are ANDed together. This is combined
// with the sub-filter, which will use the applicable operator (AND/OR/AND NOT).
func (f *filterBuilder) generateHavingClauses() (string, []interface{}) {
	clause, args := f.andClauses(f.havingClauses)

	if f.subFilter != nil {
		c, a := f.subFilter.generateHavingClauses()
		if c != "" {
			clause = f.getSubFilterClause(clause, c)
			if len(a) > 0 {
				args = append(args, a...)
			}
		}
	}

	return clause, args
}

func (f *filterBuilder) generateWithClauses() (string, []interface{}) {
	var clauses []string
	var args []interface{}
	for _, w := range f.withClauses {
		clauses = append(clauses, w.sql)
		args = append(args, w.args...)
	}

	if len(clauses) > 0 {
		return strings.Join(clauses, ", "), args
	}

	return "", nil
}

// getAllJoins returns all of the joins in this filter and any sub-filter(s).
// Redundant joins will not be duplicated in the return value.
func (f *filterBuilder) getAllJoins() joins {
	var ret joins
	ret.add(f.joins...)
	if f.subFilter != nil {
		subJoins := f.subFilter.getAllJoins()
		if len(subJoins) > 0 {
			ret.add(subJoins...)
		}
	}

	return ret
}

// getError returns the error state on this filter, or on any sub-filter(s) if
// the error state is nil.
func (f *filterBuilder) getError() error {
	if f.err != nil {
		return f.err
	}

	if f.subFilter != nil {
		return f.subFilter.getError()
	}

	return nil
}

// handleCriterion calls the handle function on the provided criterionHandler,
// providing itself.
func (f *filterBuilder) handleCriterion(ctx context.Context, handler criterionHandler) {
	handler.handle(ctx, f)
}

func (f *filterBuilder) setError(e error) {
	if f.err == nil {
		f.err = e
	}
}

func (f *filterBuilder) andClauses(input []sqlClause) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	for _, w := range input {
		clauses = append(clauses, w.sql)
		args = append(args, w.args...)
	}

	if len(clauses) > 0 {
		c := "(" + strings.Join(clauses, ") AND (") + ")"
		if len(clauses) > 1 {
			c = "(" + c + ")"
		}
		return c, args
	}

	return "", nil
}
