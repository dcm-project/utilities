package resolve

import (
	"reflect"
	"testing"
)

func TestLegacyResourceIDs(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		want []string
	}{
		{
			name: "no spec",
			body: map[string]interface{}{"run_id": "r1"},
			want: nil,
		},
		{
			name: "spec without resource_ids",
			body: map[string]interface{}{"spec": map[string]interface{}{}},
			want: nil,
		},
		{
			// Distinct from "spec without resource_ids": here the key is
			// present but holds the wrong type — a malformed-response shape
			// from the control-plane API, not just a missing field. Hits the
			// same `ok=false` branch today, but is worth pinning separately
			// since this package's job is guarding against exactly this kind
			// of untrusted-input variance.
			name: "resource_ids present with wrong type (string, not array)",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": "not-an-array"},
			},
			want: nil,
		},
		{
			name: "empty resource_ids",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": []interface{}{}},
			},
			want: nil,
		},
		{
			name: "single resource_id",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": []interface{}{"sti-1"}},
			},
			want: []string{"sti-1"},
		},
		{
			name: "multiple resource_ids, filters empty strings",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": []interface{}{"sti-1", "", "sti-2"}},
			},
			want: []string{"sti-1", "sti-2"},
		},
		{
			name: "resource_ids with non-string element",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": []interface{}{"sti-1", 42}},
			},
			want: []string{"sti-1"},
		},
		{
			// Pins the contract every call site relies on: a non-empty
			// resource_ids array where every element filters out (empty
			// strings) yields a non-nil, empty slice — NOT nil. Callers must
			// guard with len(ids) > 0, not ids != nil.
			name: "all elements filtered out yields non-nil empty slice",
			body: map[string]interface{}{
				"spec": map[string]interface{}{"resource_ids": []interface{}{"", ""}},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LegacyResourceIDs(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LegacyResourceIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestUnique(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		want    string
		wantErr bool
	}{
		{
			name:    "no candidates",
			ids:     nil,
			wantErr: true,
		},
		{
			name: "exactly one candidate",
			ids:  []string{"sti-1"},
			want: "sti-1",
		},
		{
			name:    "ambiguous - two candidates",
			ids:     []string{"sti-1", "sti-2"},
			wantErr: true,
		},
		{
			name:    "ambiguous - many candidates",
			ids:     []string{"sti-1", "sti-2", "sti-3"},
			wantErr: true,
		},
		{
			// Unreachable from today's only caller (ids come from a set's
			// keys, which can't contain duplicates), but pins the contract
			// for future callers: duplicates are NOT deduplicated, so this
			// is treated as ambiguous like any other 2-element input.
			name:    "duplicate values are ambiguous, not deduplicated",
			ids:     []string{"sti-1", "sti-1"},
			wantErr: true,
		},
		{
			// Unreachable from today's only caller (listServiceTypeInstanceIDs
			// never inserts an empty string), but pins the contract: an empty
			// string is a valid single candidate like any other value.
			name: "empty string is a valid single candidate",
			ids:  []string{""},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Unique(tt.ids)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Unique(%v) expected an error, got id=%q", tt.ids, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unique(%v) unexpected error: %v", tt.ids, err)
			}
			if got != tt.want {
				t.Errorf("Unique(%v) = %q, want %q", tt.ids, got, tt.want)
			}
		})
	}
}
