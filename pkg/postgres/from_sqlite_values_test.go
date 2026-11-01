package postgres

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertIntRange(t *testing.T) {
	_, err := convertInt(float64(1<<63), math.MinInt64, math.MaxInt64)
	assert.Error(t, err, "2^63 doesn't fit, though it compares equal to MaxInt64 as a float")

	v, err := convertInt(float64(-1<<63), math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, int64(math.MinInt64), v)

	v, err = convertInt(float64(math.MaxInt32), math.MinInt32, math.MaxInt32)
	require.NoError(t, err)
	assert.Equal(t, int64(math.MaxInt32), v)

	_, err = convertInt(float64(math.MaxInt32+1), math.MinInt32, math.MaxInt32)
	assert.Error(t, err)
	_, err = convertInt(1.5, math.MinInt32, math.MaxInt32)
	assert.Error(t, err)
}

func TestConvertJSONEscapes(t *testing.T) {
	c := copyColumn{dataType: "jsonb", nullable: true}
	for _, ok := range []string{
		`{"a":"plain"}`,
		`{"a":"\\u0000"}`, // an escaped backslash, then ordinary text
		`{"a":"\ud83d\ude00"}`,
		`{"a":"\n\u00e9"}`,
		`[1,2,3]`,
	} {
		v, err := convertJSON(ok, c)
		require.NoError(t, err, ok)
		assert.Equal(t, ok, v)
	}

	for _, bad := range []string{
		`{"a":"\u0000"}`,
		`{"a":"\\\u0000"}`, // an escaped backslash, then a real escape
		`{"a":"\ud83d"}`,
		`{"a":"\ud83d\u0041"}`,
		`{"a":"\ude00"}`,
		"{\"a\":\"\xff\"}",
		`{not json}`,
	} {
		_, err := convertJSON(bad, c)
		assert.Error(t, err, bad)
	}

	v, err := convertJSON("  ", c)
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestConvertValueRequired(t *testing.T) {
	_, err := convertValue(nil, copyColumn{dataType: "text"})
	assert.Error(t, err)

	v, err := convertValue(nil, copyColumn{dataType: "text", nullable: true})
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestConvertTimeNumbers(t *testing.T) {
	c := copyColumn{dataType: "timestamp with time zone"}
	want := time.Unix(1700000000, 0).UTC()

	for _, n := range []int64{1700000000, 1700000000000} {
		v, err := convertTime(n, c)
		require.NoError(t, err)
		assert.Equal(t, want, v)
	}

	_, err := convertTime("1700000000", c)
	assert.Error(t, err, "a number stored as text isn't something the driver reads as a date either")
}
