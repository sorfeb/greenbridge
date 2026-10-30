package ghx

import (
	"errors"
	"fmt"
)

// Fake is an in-memory GitHub for tests.
type Fake struct {
	Err       error    // returned by every call when set, for example ErrNotInstalled
	User      *Account // nil means logged out
	Addresses []Email
	EmailsErr error // returned by Emails, for example a missing scope
	Repos     map[string]Repo
}

var _ GitHub = (*Fake)(nil)

func (f *Fake) account() (Account, error) {
	if f.Err != nil {
		return Account{}, f.Err
	}
	if f.User == nil {
		return Account{}, errors.New("fake: not logged in")
	}
	return *f.User, nil
}

// Account returns User.
func (f *Fake) Account() (Account, error) { return f.account() }

// Emails returns Addresses.
func (f *Fake) Emails() ([]Email, error) {
	if _, err := f.account(); err != nil {
		return nil, err
	}
	if f.EmailsErr != nil {
		return nil, f.EmailsErr
	}
	return f.Addresses, nil
}

// Repo looks up Repos.
func (f *Fake) Repo(fullName string) (Repo, error) {
	if _, err := f.account(); err != nil {
		return Repo{}, err
	}
	r, ok := f.Repos[fullName]
	if !ok {
		return Repo{}, fmt.Errorf("fake: no repo %s", fullName)
	}
	return r, nil
}

// CreateRepo adds an empty repo under the current account.
func (f *Fake) CreateRepo(name string, private bool) error {
	u, err := f.account()
	if err != nil {
		return err
	}
	full := u.Login + "/" + name
	if _, ok := f.Repos[full]; ok {
		return fmt.Errorf("fake: %s exists", full)
	}
	if f.Repos == nil {
		f.Repos = map[string]Repo{}
	}
	f.Repos[full] = Repo{Owner: u.Login, Private: private, DefaultBranch: "main"}
	return nil
}
