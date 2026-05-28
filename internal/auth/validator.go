package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
)

type Validator struct {
	keys [][]byte
}

func NewValidator(keys []string) (*Validator, error) {
	if len(keys) == 0 {
		return nil, errors.New("auth: at least one API key is required when auth is enabled")
	}

	seen := make(map[string]struct{}, len(keys))
	parsed := make([][]byte, 0, len(keys))

	for i, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("auth: key at index %d is empty", i)
		}
		if _, dup := seen[k]; dup {
			return nil, fmt.Errorf("auth: duplicate key at index %d", i)
		}
		seen[k] = struct{}{}
		parsed = append(parsed, []byte(k))
	}

	return &Validator{keys: parsed}, nil
}

func (v *Validator) Validate(key string) bool {
	if key == "" {
		return false
	}
	candidate := []byte(key)
	var ok int // accumulate matches without short-circuiting
	for _, k := range v.keys {
		// subtle.ConstantTimeCompare returns 1 on match, 0 otherwise
		// we OR results so the loop always runs all comparisons
		ok |= subtle.ConstantTimeCompare(k, candidate)
	}
	return ok == 1
}

func (v *Validator) Len() int {
	return len(v.keys)
}
