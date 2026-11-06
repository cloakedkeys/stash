package utils

import (
	"fmt"
	"strings"
)

type StrFormatMap map[string]interface{}

// StrFormat formats the provided format string, replacing placeholders
// in the form of "{fieldName}" with the values in the provided
// StrFormatMap.
//
// For example,
//
//	StrFormat("{foo} bar {baz}", StrFormatMap{
//	    "foo": "bar",
//	    "baz": "abc",
//	})
//
// would return: "bar bar abc"
func StrFormat(format string, m StrFormatMap) string {
	args := make([]string, len(m)*2)
	i := 0

	for k, v := range m {
		args[i] = fmt.Sprintf("{%s}", k)
		args[i+1] = fmt.Sprint(v)
		i += 2
	}

	return strings.NewReplacer(args...).Replace(format)
}

// IsIntegerText reports whether s is a whole number written as an optional
// sign followed by one or more digits, and nothing else. strconv.ParseInt
// stops at the first digit that makes the number too large, so its range
// error alone doesn't show that the rest of s is digits.
func IsIntegerText(s string) bool {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// StringerSliceToStringSlice converts a slice of fmt.Stringers to a slice of strings.
func StringerSliceToStringSlice[V fmt.Stringer](v []V) []string {
	ret := make([]string, len(v))
	for i, vv := range v {
		ret[i] = vv.String()
	}

	return ret
}
