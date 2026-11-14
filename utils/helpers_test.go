package utils

import (
	"strings"
	"testing"
)

func TestHashSecret(t *testing.T) {
	hashed, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if hashed == "s3cret" {
		t.Fatal("HashSecret returned the plain secret")
	}
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("HashSecret = %q, want a bcrypt hash", hashed)
	}

	second, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if second == hashed {
		t.Fatal("HashSecret is not salted: two hashes of the same secret matched")
	}
}

func TestVerifySecret(t *testing.T) {
	hashed, err := HashSecret("s3cret")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}

	if !VerifySecret(hashed, "s3cret") {
		t.Fatal("VerifySecret rejected the correct secret")
	}
	if VerifySecret(hashed, "wrong") {
		t.Fatal("VerifySecret accepted a wrong secret")
	}
	if VerifySecret(hashed, "") {
		t.Fatal("VerifySecret accepted an empty secret")
	}
	if VerifySecret("", "s3cret") {
		t.Fatal("VerifySecret accepted an empty stored hash")
	}
}

func TestRandomString(t *testing.T) {
	t.Run("length and charset", func(t *testing.T) {
		got := RandomString(50)
		if len(got) != 50 {
			t.Fatalf("len(RandomString(50)) = %d, want 50", len(got))
		}
		for _, r := range got {
			if !strings.ContainsRune(RandomCharPool, r) {
				t.Fatalf("RandomString produced character %q outside pool", r)
			}
		}
	})

	t.Run("zero length", func(t *testing.T) {
		if got := RandomString(0); got != "" {
			t.Fatalf("RandomString(0) = %q, want empty", got)
		}
	})

	t.Run("successive calls differ", func(t *testing.T) {
		first := RandomString(50)
		second := RandomString(50)
		if first == second {
			t.Fatalf("two RandomString calls returned the same value: %q", first)
		}
	})
}
