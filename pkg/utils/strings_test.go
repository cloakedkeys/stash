package utils

import (
	"fmt"
	"testing"
)

func ExampleStrFormat() {
	fmt.Println(StrFormat("{foo} bar {baz}", StrFormatMap{
		"foo": "bar",
		"baz": "abc",
	}))
	// Output:
	// bar bar abc
}

func TestIsIntegerText(t *testing.T) {
	tests := map[string]bool{
		"0":                      true,
		"123":                    true,
		"+123":                   true,
		"-123":                   true,
		"99999999999999999999":   true,
		"-99999999999999999999":  true,
		"":                       false,
		"+":                      false,
		"-":                      false,
		"1x":                     false,
		" 1":                     false,
		"1 ":                     false,
		"0x1":                    false,
		"1.5":                    false,
		"+-1":                    false,
		"99999999999999999999 x": false,
		"99999999999999999999,0": false,
	}
	for s, want := range tests {
		if got := IsIntegerText(s); got != want {
			t.Errorf("IsIntegerText(%q) = %v, want %v", s, got, want)
		}
	}
}
