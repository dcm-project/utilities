// Package resolve contains pure, unit-tested logic for determining which
// service-type-instance ID represents the primary placement resource for a
// catalog-item-instance create/rehydrate response.
//
// It has no network dependencies and no Ginkgo/Gomega dependency, unlike the
// rest of this module (which lives under //go:build e2e and is only vetted
// and compiled in CI, never executed — see .github/workflows/validate-tests.yaml).
// This package's tests run as plain `go test`.
package resolve

import "fmt"

// LegacyResourceIDs returns spec.resource_ids when present (pre-control-plane#39
// API). Returns nil if body has no spec, or spec has no non-empty resource_ids.
func LegacyResourceIDs(body map[string]interface{}) []string {
	spec, ok := body["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := spec["resource_ids"].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Unique returns the single element of ids, or an error if ids is empty or
// contains more than one element.
//
// Candidate IDs are typically gathered from a Go map (see
// listServiceTypeInstanceIDs in the e2e package), whose iteration order is
// not deterministic. Silently returning ids[0] when there is more than one
// candidate would risk a flaky, non-deterministic result, so callers must
// treat ambiguity as an error rather than guessing (see PR #39 review
// discussion / FLPATH-4809).
//
// Two edge cases worth calling out explicitly (unreachable from today's only
// caller, which sources ids from a set's keys, but this is an exported,
// general-purpose function so future callers may hit them):
//   - Duplicates are NOT deduplicated: ids containing the same value twice
//     (e.g. []string{"a", "a"}) has len 2 and is treated as ambiguous, same
//     as any other multi-element input.
//   - An empty string is a valid candidate like any other: []string{""} has
//     len 1 and returns ("", nil). Callers that want to treat "" as "no
//     candidate" must filter it out before calling Unique.
func Unique(ids []string) (string, error) {
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("no candidate resource IDs")
	case 1:
		return ids[0], nil
	default:
		return "", fmt.Errorf("ambiguous: %d candidate resource IDs %v, expected exactly 1", len(ids), ids)
	}
}
