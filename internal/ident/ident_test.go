package ident

import (
	"bytes"
	"testing"
	"time"
)

func TestIDStableAcrossRebase(t *testing.T) {
	secret := bytes.Repeat([]byte{1}, SecretSize)
	at := time.Date(2025, 3, 4, 23, 30, 0, 0, time.FixedZone("", 7*3600))
	a := ID(secret, ModeAuthorDate, "Me@Work.com", "sha-before-rebase", at)
	b := ID(secret, ModeAuthorDate, "me@work.com", "sha-after-rebase", at)
	if a != b || len(a) != 12 {
		t.Fatalf("author-date ID changed across rebase: %q vs %q", a, b)
	}
	if ID(secret, ModeSHA, "", "x", at) == ID(secret, ModeSHA, "", "y", at) {
		t.Fatal("sha mode ignored the sha")
	}
	other := bytes.Repeat([]byte{2}, SecretSize)
	if ID(other, ModeAuthorDate, "me@work.com", "", at) == a {
		t.Fatal("ID did not depend on the secret")
	}
}

func TestStamp(t *testing.T) {
	at := time.Date(2025, 3, 4, 23, 30, 0, 0, time.FixedZone("", 7*3600))
	if got := Stamp(at, "abcdef012345", PolicyExact); !got.Equal(at) || got.Format("-0700") != "+0700" {
		t.Fatalf("exact changed the time: %v", got)
	}
	d1 := Stamp(at, "abcdef012345", PolicyDateOnly)
	d2 := Stamp(at, "abcdef012345", PolicyDateOnly)
	if !d1.Equal(d2) {
		t.Fatal("date-only not stable")
	}
	if d1.Format("2006-01-02") != "2025-03-04" || d1.Location() != time.UTC {
		t.Fatalf("date-only lost the local date: %v", d1)
	}
}

func TestRecoveryRoundTrip(t *testing.T) {
	s, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(" " + Encode(s) + "\n")
	if err != nil || !bytes.Equal(got, s) {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := Decode("abcd-efgh"); err == nil {
		t.Fatal("short phrase accepted")
	}
}
