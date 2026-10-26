package ghx

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// replay answers every gh call with a recorded response and remembers the
// arguments it was called with.
func replay(t *testing.T, fixture string) (*CLI, *[]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	var args []string
	return &CLI{run: func(a ...string) ([]byte, error) {
		args = a
		return b, nil
	}}, &args
}

func TestAccount(t *testing.T) {
	c, args := replay(t, "user.json")
	got, err := c.Account()
	if err != nil {
		t.Fatal(err)
	}
	want := Account{Login: "octocat", NoreplyEmail: "583231+octocat@users.noreply.github.com"}
	if got != want {
		t.Errorf("Account() = %+v, want %+v", got, want)
	}
	if strings.Join(*args, " ") != "api user" {
		t.Errorf("ran gh %v", *args)
	}
}

func TestAccountRejectsEmptyResponse(t *testing.T) {
	c := &CLI{run: func(...string) ([]byte, error) { return []byte(`{}`), nil }}
	if _, err := c.Account(); err == nil {
		t.Error("Account() on {} succeeded; a noreply address without an ID would never match")
	}
}

func TestEmails(t *testing.T) {
	c, args := replay(t, "emails.json")
	got, err := c.Emails()
	if err != nil {
		t.Fatal(err)
	}
	want := []Email{
		{Address: "octocat@example.com", Verified: true},
		{Address: "583231+octocat@users.noreply.github.com", Verified: true},
		{Address: "old@example.org", Verified: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Emails() = %+v, want %+v", got, want)
	}
	if strings.Join(*args, " ") != "api user/emails" {
		t.Errorf("ran gh %v", *args)
	}
}

func TestRepo(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    Repo
	}{
		{"repo_private.json", Repo{Owner: "octocat", Private: true, DefaultBranch: "main"}},
		{"repo_public_fork.json", Repo{Owner: "octocat", Fork: true, DefaultBranch: "master"}},
		{"repo_internal.json", Repo{Owner: "example-org", Private: true, DefaultBranch: "trunk"}},
		{"repo_empty.json", Repo{Owner: "octocat", Private: true}},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			c, args := replay(t, tc.fixture)
			got, err := c.Repo("octocat/greenbridge-activity")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Repo() = %+v, want %+v", got, tc.want)
			}
			if (*args)[2] != "octocat/greenbridge-activity" {
				t.Errorf("ran gh %v", *args)
			}
		})
	}
}

func TestCreateRepoVisibility(t *testing.T) {
	for _, private := range []bool{true, false} {
		var args []string
		c := &CLI{run: func(a ...string) ([]byte, error) { args = a; return nil, nil }}
		if err := c.CreateRepo("greenbridge-activity", private); err != nil {
			t.Fatal(err)
		}
		want := "--public"
		if private {
			want = "--private"
		}
		if strings.Join(args[:4], " ") != "repo create greenbridge-activity "+want {
			t.Errorf("private=%v ran gh %v", private, args)
		}
	}
}

func TestErrorsPassThrough(t *testing.T) {
	c := &CLI{run: func(...string) ([]byte, error) { return nil, ErrNotInstalled }}
	if _, err := c.Account(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Account() error %v, want ErrNotInstalled", err)
	}
	if _, err := c.Repo("a/b"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Repo() error %v, want ErrNotInstalled", err)
	}
}

func TestFakeCreateThenRepo(t *testing.T) {
	f := &Fake{User: &Account{Login: "octocat"}}
	if _, err := f.Repo("octocat/m"); err == nil {
		t.Fatal("Repo() found a repo before it was created")
	}
	if err := f.CreateRepo("m", true); err != nil {
		t.Fatal(err)
	}
	r, err := f.Repo("octocat/m")
	if err != nil || r != (Repo{Owner: "octocat", Private: true, DefaultBranch: "main"}) {
		t.Errorf("Repo() after create = %+v, %v", r, err)
	}
}
