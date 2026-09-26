package mirror

import "testing"

func TestParseSubject(t *testing.T) {
	cases := map[string]string{
		"gb/1 abcdef012345":         "abcdef012345",
		"gb/2 grp9 abcdef012345":    "abcdef012345",
		"Add README":                "",
		"gb/x abcdef012345":         "",
		"gb/1":                      "",
		"  gb/1   abcdef012345  \r": "abcdef012345",
	}
	for in, want := range cases {
		if got := ParseSubject(in); got != want {
			t.Errorf("ParseSubject(%q) = %q, want %q", in, got, want)
		}
	}
}
