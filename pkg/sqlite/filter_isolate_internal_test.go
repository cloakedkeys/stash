package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func isolateTestLevel(joinAs string, where string, args ...interface{}) *filterBuilder {
	f := &filterBuilder{}
	if joinAs != "" {
		f.addInnerJoin("scenes_tags", joinAs, joinAs+".scene_id = scenes.id")
	}
	f.addWhere(where, args...)
	return f
}

// Filters whose levels are all ANDed and don't share a join name are not
// changed (#2167, #2172).
func TestIsolateSubFilterLevelsLeavesPlainFilters(t *testing.T) {
	f := isolateTestLevel("t1", "t1.tag_id = ?", 1)
	f.addHaving("count(t1.tag_id) > ?", 2)
	sub := isolateTestLevel("t2", "t2.tag_id = ?", 3)
	f.and(sub)
	sub.and(isolateTestLevel("", "scenes.title = ?", "x"))

	f.isolateSubFilterLevels("scenes")

	assert.Len(t, f.joins, 1)
	assert.Equal(t, []sqlClause{makeClause("t1.tag_id = ?", 1)}, f.whereClauses)
	assert.Equal(t, []sqlClause{makeClause("count(t1.tag_id) > ?", 2)}, f.havingClauses)
	assert.Len(t, sub.joins, 1)
	assert.Equal(t, []sqlClause{makeClause("t2.tag_id = ?", 3)}, sub.whereClauses)
	assert.Equal(t, []sqlClause{makeClause("scenes.title = ?", "x")}, sub.subFilter.whereClauses)
}

// A level ANDed to an earlier level that uses the same join name gets its
// own subquery (#2172).
func TestIsolateSubFilterLevelsSameJoinName(t *testing.T) {
	f := isolateTestLevel("t", "t.tag_id = ?", 1)
	sub := isolateTestLevel("t", "t.tag_id = ?", 2)
	f.and(sub)

	f.isolateSubFilterLevels("scenes")

	assert.Len(t, f.joins, 1)
	assert.Equal(t, []sqlClause{makeClause("t.tag_id = ?", 1)}, f.whereClauses)
	assert.Empty(t, sub.joins)
	assert.Equal(t, []sqlClause{makeClause(
		"scenes.id IN (SELECT scenes.id FROM scenes INNER JOIN scenes_tags AS t ON t.scene_id = scenes.id WHERE (t.tag_id = ?))", 2,
	)}, sub.whereClauses)
}

// Under OR, a level keeps its inner joins inside its own subquery (#2167).
func TestIsolateSubFilterLevelsOrKeepsInnerJoins(t *testing.T) {
	f := isolateTestLevel("", "scenes.title = ?", "x")
	sub := isolateTestLevel("t", "t.tag_id = ?", 2)
	f.or(sub)

	f.isolateSubFilterLevels("scenes")

	assert.Equal(t, []sqlClause{makeClause("scenes.title = ?", "x")}, f.whereClauses)
	assert.Empty(t, sub.joins)
	assert.Equal(t, []sqlClause{makeClause(
		"scenes.id IN (SELECT scenes.id FROM scenes INNER JOIN scenes_tags AS t ON t.scene_id = scenes.id WHERE (t.tag_id = ?))", 2,
	)}, sub.whereClauses)
}

// Under NOT, a level is isolated even without joins, so a NULL column
// doesn't leave an item out of both the condition and its NOT.
func TestIsolateSubFilterLevelsNot(t *testing.T) {
	f := isolateTestLevel("", "scenes.title = ?", "x")
	sub := isolateTestLevel("", "scenes.studio_id = ?", 2)
	f.not(sub)

	f.isolateSubFilterLevels("scenes")

	assert.Equal(t, []sqlClause{makeClause("scenes.title = ?", "x")}, f.whereClauses)
	assert.Equal(t, []sqlClause{makeClause(
		"scenes.id IN (SELECT scenes.id FROM scenes WHERE (scenes.studio_id = ?))", 2,
	)}, sub.whereClauses)
}
