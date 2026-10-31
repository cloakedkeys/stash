package database

import "regexp"

const redactedPassword = "*****"

var (
	urlPassword     = regexp.MustCompile(`(://[^:/@\s]*:)\S*@`)
	keywordPassword = regexp.MustCompile(`(?i)(\bpassword\s*=\s*)('(?:[^'\\]|\\.)*'|[^\s&]*)`)
)

// RedactPath removes any password from a database path, so that the path can be
// shown. A file path comes back unchanged.
func RedactPath(path string) string {
	path = urlPassword.ReplaceAllString(path, "${1}"+redactedPassword+"@")
	return keywordPassword.ReplaceAllString(path, "${1}"+redactedPassword)
}
