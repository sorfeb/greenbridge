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
		if got := parseSubject(in); got != want {
			t.Errorf("parseSubject(%q) = %q, want %q", in, got, want)
		}
	}
}

// ls-remote lists every ref ending in the branch name; only the branch
// itself counts.
func TestRemoteTip(t *testing.T) {
	out := "1111111111111111111111111111111111111111\trefs/heads/x/main\n" +
		"2222222222222222222222222222222222222222\trefs/heads/main"
	if got := remoteTip(out, "refs/heads/main"); got != "2222222222222222222222222222222222222222" {
		t.Errorf("remoteTip(main) = %q, want the main branch's tip", got)
	}
	if got := remoteTip(out, "refs/heads/trunk"); got != "" {
		t.Errorf("remoteTip(trunk) = %q, want none", got)
	}
}
