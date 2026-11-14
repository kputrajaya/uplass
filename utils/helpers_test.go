package utils

import (
	"strings"
	"testing"
)

func TestHashString(t *testing.T) {
	const emptySHA256 = "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="
	if got := HashString(""); got != emptySHA256 {
		t.Fatalf("HashString(\"\") = %q, want %q", got, emptySHA256)
	}

	if HashString("bar") != HashString("bar") {
		t.Fatal("HashString is not deterministic")
	}
	if HashString("bar") == HashString("baz") {
		t.Fatal("HashString collides for different inputs")
	}
}

func TestRandomString(t *testing.T) {
	t.Run("length and charset", func(t *testing.T) {
		got := RandomString(50)
		if len(got) != 50 {
			t.Fatalf("len(RandomString(50)) = %d, want 50", len(got))
		}
		for _, r := range got {
			if !strings.ContainsRune(randomCharPool, r) {
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
		if RandomString(50) == RandomString(50) {
			t.Fatal("two RandomString calls returned the same value")
		}
	})
}
