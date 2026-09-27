package sql

import (
	"testing"

	"gopherbin/util"

	"golang.org/x/crypto/bcrypt"
)

// Logins for unknown accounts are verified against this hash. It must be
// computed once (hashing per request costs about twice the wrong-password
// path and gives away which usernames exist) and at the cost of real
// password hashes (so the single verification takes as long).
func TestDummyPasswordHashIsComputedOnceAtRealCost(t *testing.T) {
	first := dummyPasswordHash()
	if first == nil {
		t.Fatal("dummy hash unavailable")
	}
	if second := dummyPasswordHash(); &second[0] != &first[0] {
		t.Fatal("dummy hash recomputed on every call")
	}
	real, err := util.PaswsordToBcrypt("x")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	want, _ := bcrypt.Cost([]byte(real))
	if got, err := bcrypt.Cost(first); err != nil || got != want {
		t.Fatalf("dummy hash cost = %d (%v), want %d", got, err, want)
	}
}
