package postgres

import (
	"fmt"
	"unicode/utf8"
)

// maxLogArgBytes is how much of a long value a log line shows.
const maxLogArgBytes = 200

// logArgs shortens a query's long values for the log, such as a whole story's text,
// so a log line stays readable and doesn't copy what the library holds.
func logArgs(args []interface{}) []interface{} {
	ret := make([]interface{}, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			ret[i] = shortenForLog(v)
		case []byte:
			if len(v) > maxLogArgBytes {
				ret[i] = fmt.Sprintf("<%d bytes>", len(v))
			} else {
				ret[i] = v
			}
		default:
			ret[i] = a
		}
	}
	return ret
}

func shortenForLog(s string) string {
	if len(s) <= maxLogArgBytes {
		return s
	}
	n := maxLogArgBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return fmt.Sprintf("%s… (%d bytes)", s[:n], len(s))
}
