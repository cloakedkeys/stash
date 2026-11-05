package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
)

const maxCustomFieldNameLength = 64

type customFieldsStore struct {
	table exp.IdentifierExpression
	fk    exp.IdentifierExpression
}

func (s *customFieldsStore) deleteForID(ctx context.Context, id int) error {
	table := s.table
	q := dialect.Delete(table).Where(s.fk.Eq(id))
	_, err := exec(ctx, q)
	if err != nil {
		return fmt.Errorf("deleting from %s: %w", s.table.GetTable(), err)
	}

	return nil
}

func (s *customFieldsStore) SetCustomFields(ctx context.Context, id int, values models.CustomFieldsInput) error {
	var partial bool
	var valMap map[string]interface{}

	switch {
	case values.Full != nil:
		partial = false
		valMap = values.Full
	case values.Partial != nil:
		partial = true
		valMap = values.Partial
	}

	if valMap != nil {
		if err := s.validateCustomFields(valMap, values.Remove); err != nil {
			return err
		}

		if err := s.setCustomFields(ctx, id, valMap, partial); err != nil {
			return err
		}
	}

	if err := s.deleteCustomFields(ctx, id, values.Remove); err != nil {
		return err
	}

	return nil
}

func (s *customFieldsStore) validateCustomFields(values map[string]interface{}, deleteKeys []string) error {
	// if values is nil, nothing to validate
	if values == nil {
		return nil
	}

	// ensure that custom field names are valid
	// no leading or trailing whitespace, no empty strings
	for k := range values {
		if err := s.validateCustomFieldName(k); err != nil {
			return fmt.Errorf("custom field name %q: %w", k, err)
		}
	}

	// ensure delete keys are not also in values
	for _, k := range deleteKeys {
		if _, ok := values[k]; ok {
			return fmt.Errorf("custom field name %q cannot be in both values and delete keys", k)
		}
	}

	return nil
}

func (s *customFieldsStore) validateCustomFieldName(fieldName string) error {
	// ensure that custom field names are valid
	// no leading or trailing whitespace, no empty strings
	if strings.TrimSpace(fieldName) == "" {
		return fmt.Errorf("custom field name cannot be empty")
	}
	if fieldName != strings.TrimSpace(fieldName) {
		return fmt.Errorf("custom field name cannot have leading or trailing whitespace")
	}
	if len(fieldName) > maxCustomFieldNameLength {
		return fmt.Errorf("custom field name must be less than %d characters", maxCustomFieldNameLength+1)
	}
	return nil
}

func getSQLValueFromCustomFieldInput(input any) (interface{}, error) {
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal custom field value: %w", err)
	}
	return string(jsonBytes), nil
}

func getSQLTypeFromInput(input any) string {
	return reflect.TypeOf(input).String()
}

func (s *customFieldsStore) sqlValueToValue(value interface{}, gotype string) (interface{}, error) {
	val, ok := value.([]byte)
	if !ok {
		return value, nil
	}

	var res interface{}

	if err := json.Unmarshal(val, &res); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSONB value: %w", err)
	}

	switch gotype {
	case "string":
		str, ok := res.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", res)
		}
		return str, nil
	case "int64":
		// the exact digits, because a float64 rounds numbers above 2^53
		if i, err := strconv.ParseInt(string(val), 10, 64); err == nil {
			return i, nil
		}
		f, ok := res.(float64)
		if !ok {
			return nil, fmt.Errorf("expected float64 for int64 conversion, got %T", res)
		}
		return int64(f), nil
	case "float64":
		f, ok := res.(float64)
		if !ok {
			return nil, fmt.Errorf("expected float64, got %T", res)
		}
		return f, nil
	case "[]interface {}":
		arr, ok := res.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected array, got %T", res)
		}
		return arr, nil
	case "map[string]interface {}":
		m, ok := res.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected map, got %T", res)
		}
		return m, nil
	default:
		return res, nil
	}
}

func (s *customFieldsStore) setCustomFields(ctx context.Context, id int, values map[string]interface{}, partial bool) error {
	if !partial {
		// delete existing custom fields
		if err := s.deleteForID(ctx, id); err != nil {
			return err
		}
	}

	if len(values) == 0 {
		return nil
	}

	conflictKey := s.fk.GetCol().(string) + ", field"
	// upsert new custom fields
	q := dialect.Insert(s.table).Prepared(true).Cols(s.fk, "field", "value", "type").
		OnConflict(goqu.DoUpdate(conflictKey, goqu.Record{"value": goqu.I("excluded.value"), "type": goqu.I("excluded.type")}))
	r := make([]interface{}, len(values))
	var i int
	for key, value := range values {
		v, err := getSQLValueFromCustomFieldInput(value)
		if err != nil {
			return fmt.Errorf("getting SQL value for field %q: %w", key, err)
		}
		r[i] = goqu.Record{
			"field":                key,
			"value":                goqu.L("?::JSONB", v),
			"type":                 getSQLTypeFromInput(value),
			s.fk.GetCol().(string): id,
		}
		i++
	}

	if _, err := exec(ctx, q.Rows(r...)); err != nil {
		return fmt.Errorf("inserting custom fields: %w", err)
	}

	return nil
}

func (s *customFieldsStore) deleteCustomFields(ctx context.Context, id int, keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	q := dialect.Delete(s.table).
		Where(s.fk.Eq(id)).
		Where(goqu.I("field").In(keys))

	if _, err := exec(ctx, q); err != nil {
		return fmt.Errorf("deleting custom fields: %w", err)
	}

	return nil
}

func (s *customFieldsStore) GetCustomFields(ctx context.Context, id int) (map[string]interface{}, error) {
	q := dialect.Select("field", "value", "type").From(s.table).Prepared(true).Where(s.fk.Eq(id))

	const single = false
	ret := make(map[string]interface{})
	err := queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
		var err error
		var field string
		var value interface{}
		var gotype string
		if err = rows.Scan(&field, &value, &gotype); err != nil {
			return fmt.Errorf("scanning custom fields: %w", err)
		}
		ret[field], err = s.sqlValueToValue(value, gotype)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("getting custom fields: %w", err)
	}

	return ret, nil
}

func (s *customFieldsStore) GetCustomFieldsBulk(ctx context.Context, ids []int) ([]models.CustomFieldMap, error) {
	q := dialect.Select(s.fk.As("id"), "field", "value", "type").From(s.table).Where(s.fk.In(ids))

	const single = false
	ret := make([]models.CustomFieldMap, len(ids))
	// initialise ret with empty maps for each id
	for i := range ret {
		ret[i] = make(map[string]interface{})
	}

	idi := idToIndexMap(ids)

	err := queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
		var err error
		var id int
		var field string
		var value interface{}
		var gotype string
		if err = rows.Scan(&id, &field, &value, &gotype); err != nil {
			return fmt.Errorf("scanning custom fields: %w", err)
		}

		i := idi[id]
		m := ret[i]
		if m == nil {
			m = make(map[string]interface{})
			ret[i] = m
		}

		m[field], err = s.sqlValueToValue(value, gotype)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("getting custom fields: %w", err)
	}

	sliceutil.FillRepeats(ret, ids)

	return ret, nil
}

type customFieldsFilterHandler struct {
	table string
	fkCol string
	c     []models.CustomFieldCriterionInput
	idCol string
}

func (h *customFieldsFilterHandler) innerJoin(f *filterBuilder, as string, field string) {
	joinOn := fmt.Sprintf("%s = %s.%s AND %s.field = ?", h.idCol, as, h.fkCol, as)
	f.addInnerJoin(h.table, as, joinOn, field)
}

func (h *customFieldsFilterHandler) leftJoin(f *filterBuilder, as string, field string) {
	joinOn := fmt.Sprintf("%s = %s.%s AND %s.field = ?", h.idCol, as, h.fkCol, as)
	f.addLeftJoin(h.table, as, joinOn, field)
}

// customFieldStrings gives values as text, to compare with a stored value as text.
func customFieldStrings(values []interface{}) []interface{} {
	ret := make([]interface{}, len(values))
	for i, v := range values {
		ret[i] = fmt.Sprint(v)
	}
	return ret
}

// sqliteFloatText is the SQL for a number as SQLite writes a REAL as text:
// "1.0", "0.0001", "1.0e-05", "1.0e+20". SQLite uses at most 17 significant
// digits, and the plain form unless the exponent is below -4 or above 16. A
// float is stored here with the fewest digits that read back as the same
// float, and this lays those digits out as SQLite does. For a float typed with
// up to 13 significant digits SQLite almost always writes those same digits;
// with more, or for a few starting with 9 (9.59e-10), it can write up to 17,
// and the two can still differ.
//
// s is the sign, i and f the digits before and after the point, d the
// significant digits and e the exponent.
const sqliteFloatText = `(SELECT CASE
	WHEN d = '' THEN '0.0'
	WHEN e < -4 OR e > 16 THEN s || left(d, 1) || '.' || COALESCE(NULLIF(substr(d, 2), ''), '0') ||
		'e' || CASE WHEN e < 0 THEN '-' ELSE '+' END || CASE WHEN abs(e) < 10 THEN '0' ELSE '' END || abs(e)
	WHEN e < 0 THEN s || '0.' || repeat('0', -e - 1) || d
	WHEN length(d) <= e + 1 THEN s || d || repeat('0', e + 1 - length(d)) || '.0'
	ELSE s || left(d, e + 1) || '.' || substr(d, e + 2)
END FROM (
	SELECT s, rtrim(ltrim(i || f, '0'), '0') AS d,
		CASE WHEN i <> '0' THEN length(i) - 1 ELSE length(ltrim(f, '0')) - length(f) - 1 END AS e
	FROM (
		SELECT CASE WHEN n < 0 THEN '-' ELSE '' END AS s,
			split_part(abs(n)::text, '.', 1) AS i, split_part(abs(n)::text, '.', 2) AS f
		FROM (SELECT %[1]s.value::numeric AS n) AS cf_number
	) AS cf_parts
) AS cf_digits)`

// customFieldText is the SQL for the custom field value of joinAs as text, the
// way SQLite reads it, so that includes, excludes and regex filters find the
// same items on both databases. SQLite stores true and false as 1 and 0, and a
// float as a REAL; other values read the same on both.
func customFieldText(joinAs string) string {
	return fmt.Sprintf(`(CASE
	WHEN jsonb_typeof(%[1]s.value) = 'boolean' THEN CASE WHEN %[1]s.value = 'true'::jsonb THEN '1' ELSE '0' END
	WHEN jsonb_typeof(%[1]s.value) = 'number' AND %[1]s.type = 'float64' THEN `+sqliteFloatText+`
	ELSE %[1]s.value->>0
END)`, joinAs)
}

// customFieldSearchText is a criterion value as text for includes and
// excludes, written as SQLite's store writes it: a JSON number becomes an
// integer or a float first, so 1.0 searches for "1".
func customFieldSearchText(v interface{}) string {
	if n, ok := v.(json.Number); ok {
		if num, err := models.JSONNumberValue(n); err == nil {
			return fmt.Sprint(num)
		}
	}
	return fmt.Sprint(v)
}

// checkCustomFieldValueCount sets an error on f, naming the modifier, unless cv
// holds exactly want values, or at least want when atLeast is set. A modifier
// that reads cv[0] or cv[1] calls this first, so a short list is refused rather
// than indexed out of range.
func checkCustomFieldValueCount(f *filterBuilder, modifier models.CriterionModifier, cv []interface{}, want int, atLeast bool) bool {
	if len(cv) == want || (atLeast && len(cv) > want) {
		return true
	}
	noun := "values"
	if want == 1 {
		noun = "value"
	}
	if atLeast {
		noun += " or more"
	}
	f.setError(fmt.Errorf("expected %d %s for custom field criterion modifier %s, got %d", want, noun, modifier, len(cv)))
	return false
}

func (h *customFieldsFilterHandler) handleCriterion(f *filterBuilder, joinAs string, cc models.CustomFieldCriterionInput) {
	// convert values
	cv := append([]interface{}{}, cc.Value...)

	// equals and not equals compare this, as before
	valueAsString := fmt.Sprintf("%s.value->>0", joinAs)
	valueAsText := customFieldText(joinAs)
	valueAsNumber := fmt.Sprintf("%s.value::numeric", joinAs)
	valueIsNumber := fmt.Sprintf("jsonb_typeof(%s.value) = 'number'", joinAs)

	switch cc.Modifier {
	case models.CriterionModifierEquals:
		h.innerJoin(f, joinAs, cc.Field)
		if len(cv) == 0 {
			// PostgreSQL has no empty IN list; SQLite reads IN () as false, so
			// equals with no values finds nothing there (#2067)
			f.addWhere("FALSE")
			break
		}
		f.addWhere(fmt.Sprintf("%s IN %s", valueAsString, getInBinding(len(cv))), customFieldStrings(cv)...)
	case models.CriterionModifierNotEquals:
		h.leftJoin(f, joinAs, cc.Field)
		if len(cv) == 0 {
			// and NOT IN () as true, so not equals with no values finds
			// everything
			break
		}
		f.addWhere(fmt.Sprintf("(%s NOT IN %s OR %s.value IS NULL)", valueAsString, getInBinding(len(cv)), joinAs), customFieldStrings(cv)...)
	case models.CriterionModifierIncludes:
		clauses := make([]sqlClause, len(cv))
		for i, v := range cv {
			clauses[i] = makeClause(fmt.Sprintf("%s ILIKE ? ESCAPE '\\'", valueAsText), like(customFieldSearchText(v)))
		}
		h.innerJoin(f, joinAs, cc.Field)
		f.whereClauses = append(f.whereClauses, clauses...)
	case models.CriterionModifierExcludes:
		for _, v := range cv {
			f.addWhere(fmt.Sprintf("(%s NOT ILIKE ? ESCAPE '\\' OR %s.value IS NULL)", valueAsText, joinAs), like(customFieldSearchText(v)))
		}
		h.leftJoin(f, joinAs, cc.Field)
	case models.CriterionModifierMatchesRegex:
		for _, v := range cv {
			vs, err := models.CustomFieldRegexPattern(v)
			if err != nil {
				f.setError(err)
				return
			}
			translated, err := translateRegex(vs)
			if err != nil {
				f.setError(err)
				return
			}
			f.addWhere(fmt.Sprintf("(%s) ~ ?", valueAsText), translated)
		}
		h.innerJoin(f, joinAs, cc.Field)
	case models.CriterionModifierNotMatchesRegex:
		for _, v := range cv {
			vs, err := models.CustomFieldRegexPattern(v)
			if err != nil {
				f.setError(err)
				return
			}
			translated, err := translateRegex(vs)
			if err != nil {
				f.setError(err)
				return
			}
			f.addWhere(fmt.Sprintf("(%s.value IS NULL OR (%s) !~ ?)", joinAs, valueAsText), translated)
		}
		h.leftJoin(f, joinAs, cc.Field)
	case models.CriterionModifierIsNull:
		h.leftJoin(f, joinAs, cc.Field)
		f.addWhere(fmt.Sprintf("%s.value IS NULL OR TRIM(%s) = ''", joinAs, valueAsString))
	case models.CriterionModifierNotNull:
		h.innerJoin(f, joinAs, cc.Field)
		f.addWhere(fmt.Sprintf("TRIM(%s) != ''", valueAsString))
	case models.CriterionModifierBetween:
		if !checkCustomFieldValueCount(f, cc.Modifier, cv, 2, false) {
			return
		}
		h.innerJoin(f, joinAs, cc.Field)
		f.addWhere(valueIsNumber)
		f.addWhere(fmt.Sprintf("%s BETWEEN ? AND ?", valueAsNumber), cv[0], cv[1])
	case models.CriterionModifierNotBetween:
		// two or more, using the first two, unlike between: upstream took any
		// number here and only fewer than two failed, so saved filters with more
		// keep working
		if !checkCustomFieldValueCount(f, cc.Modifier, cv, 2, true) {
			return
		}
		h.leftJoin(f, joinAs, cc.Field)
		f.addWhere(fmt.Sprintf("((%s AND %s NOT BETWEEN ? AND ?) OR %s.value IS NULL)", valueIsNumber, valueAsNumber, joinAs), cv[0], cv[1])
	case models.CriterionModifierLessThan:
		if !checkCustomFieldValueCount(f, cc.Modifier, cv, 1, false) {
			return
		}
		h.innerJoin(f, joinAs, cc.Field)
		f.addWhere(valueIsNumber)
		f.addWhere(fmt.Sprintf("%s < ?", valueAsNumber), cv[0])
	case models.CriterionModifierGreaterThan:
		if !checkCustomFieldValueCount(f, cc.Modifier, cv, 1, false) {
			return
		}
		h.innerJoin(f, joinAs, cc.Field)
		f.addWhere(valueIsNumber)
		f.addWhere(fmt.Sprintf("%s > ?", valueAsNumber), cv[0])
	default:
		f.setError(fmt.Errorf("unsupported custom field criterion modifier: %s", cc.Modifier))
	}
}

func (h *customFieldsFilterHandler) handle(ctx context.Context, f *filterBuilder) {
	if len(h.c) == 0 {
		return
	}

	for i, cc := range h.c {
		join := fmt.Sprintf("custom_fields_%d", i)
		h.handleCriterion(f, join, cc)
	}
}
