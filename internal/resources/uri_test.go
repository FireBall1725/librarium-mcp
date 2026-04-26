// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import "testing"

func TestMatchURI(t *testing.T) {
	cases := []struct {
		name     string
		uri      string
		template string
		wantOK   bool
		want     map[string]string
	}{
		{
			name:     "single param",
			uri:      "librarium://library/abc-123",
			template: "librarium://library/{id}",
			wantOK:   true,
			want:     map[string]string{"id": "abc-123"},
		},
		{
			name:     "two params",
			uri:      "librarium://library/lib-1/series/ser-2",
			template: "librarium://library/{lib}/series/{sid}",
			wantOK:   true,
			want:     map[string]string{"lib": "lib-1", "sid": "ser-2"},
		},
		{
			name:     "wrong segment count",
			uri:      "librarium://library/abc/series/xyz",
			template: "librarium://library/{id}",
			wantOK:   false,
		},
		{
			name:     "literal mismatch",
			uri:      "librarium://library/abc/books",
			template: "librarium://library/{id}/series",
			wantOK:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := matchURI(tc.uri, tc.template)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %s = %q want %q", k, got[k], v)
				}
			}
		})
	}
}
