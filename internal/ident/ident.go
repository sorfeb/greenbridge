// Package ident turns commit metadata into opaque IDs and applies the time
// policy. Raw hashes never reach the mirror: anyone with work repo access
// could match them to your commits.
package ident

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Identity modes.
const (
	ModeAuthorDate = "author-date" // survives rebase, cherry-pick, amend
	ModeSHA        = "sha"
)

// Time policies.
const (
	PolicyExact    = "exact"     // keep timestamp and UTC offset
	PolicyDateOnly = "date-only" // keep local date, hour derived from ID
)

// ID is HMAC(secret, email NUL author_unix)[:12] in author-date mode, or
// HMAC(secret, sha)[:12] in sha mode.
func ID(secret []byte, mode, email, sha string, t time.Time) string {
	msg := strings.ToLower(email) + "\x00" + strconv.FormatInt(t.Unix(), 10)
	if mode == ModeSHA {
		msg = sha
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))[:12]
}

// Stamp returns the time written to the mirror for an activity.
// date-only keeps the calendar date in the commit's own offset and replaces
// the time of day with one derived from the ID, so reruns are stable.
func Stamp(t time.Time, id, policy string) time.Time {
	if policy != PolicyDateOnly {
		return t
	}
	n, _ := strconv.ParseUint(id[:6], 16, 32)
	y, mo, d := t.Date()
	return time.Date(y, mo, d, int(n%24), int(n/24%60), 0, 0, time.UTC)
}

// SecretSize is the secret length in bytes.
const SecretSize = 32

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random secret.
func NewSecret() ([]byte, error) {
	s := make([]byte, SecretSize)
	_, err := rand.Read(s)
	return s, err
}

// Encode renders a secret as a recovery phrase: lowercase base32 in
// dash-separated groups of four.
func Encode(secret []byte) string {
	raw := strings.ToLower(b32.EncodeToString(secret))
	var groups []string
	for len(raw) > 4 {
		groups = append(groups, raw[:4])
		raw = raw[4:]
	}
	return strings.Join(append(groups, raw), "-")
}

// Decode parses a recovery phrase, ignoring dashes, spaces, and case.
func Decode(phrase string) ([]byte, error) {
	clean := strings.ToUpper(strings.NewReplacer("-", "", " ", "", "\n", "", "\r", "").Replace(phrase))
	s, err := b32.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid recovery phrase: %w", err)
	}
	if len(s) != SecretSize {
		return nil, fmt.Errorf("invalid recovery phrase: got %d bytes, want %d", len(s), SecretSize)
	}
	return s, nil
}

// ErrNoSecret is wrapped by Load when the secret file does not exist.
var ErrNoSecret = errors.New("run gbridge init")

// Load reads a secret file.
func Load(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no secret at %s: %w", path, ErrNoSecret)
	}
	if err != nil {
		return nil, err
	}
	return Decode(string(b))
}

// Save writes a secret file readable only by the user.
func Save(path string, secret []byte) error {
	return os.WriteFile(path, []byte(Encode(secret)+"\n"), 0o600)
}
