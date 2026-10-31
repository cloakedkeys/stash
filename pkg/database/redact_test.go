package database

import "testing"

func TestRedactPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"", ""},
		{"/stash/stash-go.sqlite", "/stash/stash-go.sqlite"},
		{`C:\stash\stash-go.sqlite`, `C:\stash\stash-go.sqlite`},
		{"postgresql://user@host/db", "postgresql://user@host/db"},
		{"postgresql://user:secret@host:5432/db?sslmode=disable", "postgresql://user:*****@host:5432/db?sslmode=disable"},
		{"POSTGRESQL:postgresql://user:se@cret@host/db", "POSTGRESQL:postgresql://user:*****@host/db"},
		{"postgresql://u:pw@host/db?application_name=a@b", "postgresql://u:*****@host/db?application_name=a@b"},
		{"postgresql://host/db?user=me&password=secret&sslmode=disable", "postgresql://host/db?user=me&password=*****&sslmode=disable"},
		{"host=h user=u password=secret dbname=d", "host=h user=u password=***** dbname=d"},
		{"host=h password='se cr\\'et' dbname=d", "host=h password=***** dbname=d"},
	}

	for _, tt := range tests {
		if got := RedactPath(tt.path); got != tt.want {
			t.Errorf("RedactPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
