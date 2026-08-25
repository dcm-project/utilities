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
