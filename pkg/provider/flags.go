// Package provider holds pure helpers for the `provider` command family:
// flag parsing and provider-spec construction/merging. No I/O, no gateway
// calls — see internal/cli/provider.go for the command wiring that calls
// these.
package provider

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseCredentialPairs parses --credential items. Each item is either
// KEY=VALUE (value used as-is, may itself contain '='), or a bare KEY, which
// is looked up via getenv (the Rust CLI's "env lookup key" form). A later
// duplicate key overwrites an earlier one.
func ParseCredentialPairs(items []string, getenv func(string) string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range items {
		key, value, hasValue := strings.Cut(item, "=")
		if key == "" {
			return nil, fmt.Errorf("--credential key cannot be empty, got %q", item)
		}
		if !hasValue {
			value = getenv(key)
			if value == "" {
				return nil, fmt.Errorf("--credential %s: no value given and %s is not set in the environment", key, key)
			}
		}
		out[key] = value
	}
	return out, nil
}

// ParseConfigPairs parses --config KEY=VALUE items. Unlike
// ParseCredentialPairs, there is no bare-KEY env-lookup form — config values
// are never secret, so an explicit value is always required.
func ParseConfigPairs(items []string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range items {
		key, value, hasValue := strings.Cut(item, "=")
		if !hasValue {
			return nil, fmt.Errorf("--config expects KEY=VALUE, got %q", item)
		}
		if key == "" {
			return nil, fmt.Errorf("--config key cannot be empty, got %q", item)
		}
		out[key] = value
	}
	return out, nil
}

// ParseCredentialExpiresAt parses --credential-expires-at KEY=TIMESTAMP
// items. TIMESTAMP is either all-digit epoch milliseconds or an RFC3339
// timestamp. A timestamp of exactly 0 means "clear this credential's
// expiry" and is represented as the zero time.Time — callers (MergeSpec)
// interpret that as a deletion, not a literal expiry of the Unix epoch.
func ParseCredentialExpiresAt(items []string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, item := range items {
		key, value, hasValue := strings.Cut(item, "=")
		if !hasValue {
			return nil, fmt.Errorf("--credential-expires-at expects KEY=TIMESTAMP, got %q", item)
		}
		if key == "" {
			return nil, fmt.Errorf("--credential-expires-at key cannot be empty, got %q", item)
		}
		t, err := parseTimestamp(value)
		if err != nil {
			return nil, fmt.Errorf("--credential-expires-at %s: %w", key, err)
		}
		out[key] = t
	}
	return out, nil
}

// parseTimestamp accepts epoch milliseconds (all-digit) or RFC3339.
func parseTimestamp(value string) (time.Time, error) {
	if isAllDigits(value) {
		ms, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		if ms == 0 {
			return time.Time{}, nil
		}
		return time.UnixMilli(ms).UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("not a valid epoch-milliseconds or RFC3339 timestamp: %q", value)
	}
	return t, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
