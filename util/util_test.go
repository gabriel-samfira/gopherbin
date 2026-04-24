package util_test

import (
	"strings"
	"testing"
	"unicode"

	"gopherbin/util"
)

func TestIsValidEmail(t *testing.T) {
	cases := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"user.name+tag@domain.co.uk", true},
		{"", false},
		{"not-an-email", false},
		{"@no-local-part.com", false},
		{"no-at-sign", false},
		{strings.Repeat("a", 245) + "@example.com", false}, // > 254 chars
	}
	for _, tc := range cases {
		if got := util.IsValidEmail(tc.email); got != tc.valid {
			t.Errorf("IsValidEmail(%q) = %v, want %v", tc.email, got, tc.valid)
		}
	}
}

func TestIsAlphanumeric(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"abc123", true},
		{"ABC", true},
		{"", true}, // vacuous truth
		{"abc def", false},
		{"abc-def", false},
		{"abc!", false},
	}
	for _, tc := range cases {
		if got := util.IsAlphanumeric(tc.input); got != tc.want {
			t.Errorf("IsAlphanumeric(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestGetRandomString_LengthAndCharset(t *testing.T) {
	const n = 24
	s, err := util.GetRandomString(n)
	if err != nil {
		t.Fatalf("GetRandomString: %v", err)
	}
	if len(s) != n {
		t.Fatalf("expected length %d, got %d", n, len(s))
	}
	for _, c := range s {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			t.Fatalf("unexpected character %q in random string", c)
		}
	}
}

func TestGetRandomString_Uniqueness(t *testing.T) {
	a, err := util.GetRandomString(32)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := util.GetRandomString(32)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if a == b {
		t.Fatalf("expected different strings, both returned %q", a)
	}
}

func TestHashString_Deterministic(t *testing.T) {
	h1, err := util.HashString("hello")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	h2, err := util.HashString("hello")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("non-deterministic: %d != %d", h1, h2)
	}
}

func TestHashString_DifferentInputs(t *testing.T) {
	h1, _ := util.HashString("hello")
	h2, _ := util.HashString("world")
	if h1 == h2 {
		t.Fatal("collision on different inputs")
	}
}
