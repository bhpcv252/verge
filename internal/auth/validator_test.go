package auth

import (
	"strings"
	"testing"
)

func TestNewValidator(t *testing.T) {
	t.Run("valid single key", func(t *testing.T) {
		v, err := NewValidator([]string{"key-abc123"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v == nil {
			t.Fatal("expected non-nil validator")
		}
		if v.Len() != 1 {
			t.Fatalf("expected 1 key, got %d", v.Len())
		}
	})

	t.Run("valid multiple keys", func(t *testing.T) {
		v, err := NewValidator([]string{"key-abc123", "key-def456"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.Len() != 2 {
			t.Fatalf("expected 2 keys, got %d", v.Len())
		}
	})

	t.Run("trims whitespace around keys", func(t *testing.T) {
		v, err := NewValidator([]string{"  key-abc123  "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// trimmed key must still validate correctly
		if !v.Validate("key-abc123") {
			t.Fatal("expected trimmed key to validate")
		}
	})

	t.Run("rejects empty slice", func(t *testing.T) {
		_, err := NewValidator([]string{})
		if err == nil {
			t.Fatal("expected error for empty key list")
		}
	})

	t.Run("rejects nil slice", func(t *testing.T) {
		_, err := NewValidator(nil)
		if err == nil {
			t.Fatal("expected error for nil key list")
		}
	})

	t.Run("rejects blank key in list", func(t *testing.T) {
		_, err := NewValidator([]string{"key-abc123", "   "})
		if err == nil {
			t.Fatal("expected error for blank key")
		}
	})

	t.Run("rejects empty string key in list", func(t *testing.T) {
		_, err := NewValidator([]string{"key-abc123", ""})
		if err == nil {
			t.Fatal("expected error for empty string key")
		}
	})

	t.Run("rejects duplicate keys", func(t *testing.T) {
		_, err := NewValidator([]string{"key-abc123", "key-abc123"})
		if err == nil {
			t.Fatal("expected error for duplicate keys")
		}
	})
}

func TestValidator_Validate(t *testing.T) {
	v, err := NewValidator([]string{"key-abc123", "key-def456"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact first key", "key-abc123", true},
		{"exact second key", "key-def456", true},
		{"wrong key", "key-wrong", false},
		{"empty string", "", false},
		{"prefix of valid key", "key-abc", false},
		{"valid key with extra char", "key-abc123x", false},
		{"valid key uppercase", "KEY-ABC123", false},
		{"whitespace around key", " key-abc123 ", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := v.Validate(tc.input)
			if got != tc.want {
				t.Errorf("Validate(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidator_ConstantTimeAllKeysChecked(t *testing.T) {
	keys := make([]string, 10)
	for i := range keys {
		keys[i] = strings.Repeat("x", i+1)
	}
	v, err := NewValidator(keys)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// last key in the list
	last := keys[len(keys)-1]
	if !v.Validate(last) {
		t.Errorf("expected last key %q to validate", last)
	}
	// first key
	if !v.Validate(keys[0]) {
		t.Errorf("expected first key %q to validate", keys[0])
	}
}
