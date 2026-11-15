package localaccounts

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

var (
	testHashOnce sync.Once
	testHash     string
	testHashErr  error
)

func validTestHash(t *testing.T) string {
	t.Helper()
	testHashOnce.Do(func() {
		testHash, testHashErr = HashPassword("correct-horse-battery")
	})
	if testHashErr != nil {
		t.Fatalf("create test password hash: %v", testHashErr)
	}
	return testHash
}

func TestPasswordHashRoundTripAndRandomSalt(t *testing.T) {
	first := validTestHash(t)
	second, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected Argon2id encoding: %s", first)
	}
	if first == second {
		t.Fatal("password hashes reused a salt")
	}
	if !verifyPassword(first, "correct-horse-battery") || !verifyPassword(second, "correct-horse-battery") {
		t.Fatal("valid password rejected")
	}
	if verifyPassword(first, "wrong-password") {
		t.Fatal("wrong password accepted")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	valid := validTestHash(t)
	parts := strings.Split(valid, "$")
	tests := map[string]string{
		"empty":              "",
		"wrong algorithm":    strings.Replace(valid, "argon2id", "argon2i", 1),
		"wrong version":      strings.Replace(valid, "v=19", "v=16", 1),
		"invalid parameters": strings.Replace(valid, "m=19456", "m=0", 1),
		"excessive memory":   strings.Replace(valid, "m=19456", "m=1073741824", 1),
		"invalid salt":       strings.Replace(valid, parts[4], "%%%", 1),
		"short salt":         strings.Replace(valid, parts[4], "YWJj", 1),
		"invalid digest":     strings.Replace(valid, parts[5], "%%%", 1),
		"trailing data":      valid + "$extra",
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			if verifyPassword(encoded, "correct-horse-battery") {
				t.Fatal("malformed or unsafe hash accepted")
			}
		})
	}
}

func TestHashPasswordValidatesLength(t *testing.T) {
	for _, password := range []string{"", "too-short", strings.Repeat("x", 129)} {
		if _, err := HashPassword(password); !errors.Is(err, ErrValidation) {
			t.Fatalf("password length %d: expected validation error, got %v", len(password), err)
		}
	}
}
