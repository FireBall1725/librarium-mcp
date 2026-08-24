// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fireball1725/librarium-mcp/internal/version"
)

// TestEveryRequestIdentifiesTheClient pins the two headers the server's version
// gate keys on.
//
// Without them a request is taken for a third-party integration and waved
// through, which is the one thing MCP must not be: its reading-state tools are
// the part the tier schema changes, so it has to be checked like any other
// first-party client.
func TestEveryRequestIdentifiesTheClient(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		var gotName, gotVersion string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotName = r.Header.Get("X-Librarium-Client")
			gotVersion = r.Header.Get("X-Librarium-Client-Version")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))

		c := New(srv.URL, "lbrm_pat_test")
		if _, err := c.doRaw(context.Background(), method, "/api/v1/anything", nil); err != nil {
			srv.Close()
			t.Fatalf("%s: %v", method, err)
		}
		srv.Close()

		if gotName != "mcp" {
			t.Errorf("%s sent client %q, want mcp", method, gotName)
		}
		if gotVersion != version.BuildVersion {
			t.Errorf("%s sent version %q, want %q", method, gotVersion, version.BuildVersion)
		}
		// A local build must still read as a version, or the server cannot tell
		// "developer" apart from "not a version at all" and fails closed on it.
		if gotVersion == "" {
			t.Errorf("%s sent an empty version", method)
		}
	}
}
