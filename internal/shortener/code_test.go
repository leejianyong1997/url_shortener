package shortener

import (
	"strings"
	"testing"
)

// base62 is the expected alphabet: digits + lower + upper = 62 symbols.
const base62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func TestGenerateCodeHasRequestedLength(t *testing.T) {
	code, err := GenerateCode(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(code) != 7 {
		t.Errorf("got length %d, want 7", len(code))
	}
}

func TestGenerateCodeUsesOnlyBase62(t *testing.T) {
	code, err := GenerateCode(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, c := range code {
		if !strings.ContainsRune(base62, c) {
			t.Errorf("code contains non-base62 character %q", c)
		}
	}
}

func TestGenerateCodeIsNotConstant(t *testing.T) {
	a, _ := GenerateCode(7)
	b, _ := GenerateCode(7)
	if a == b {
		t.Errorf("two consecutive codes were identical (%q) — not random", a)
	}
}
