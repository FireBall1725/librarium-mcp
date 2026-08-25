// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listLoans drives list_loans and reports the request it made. The two shapes
// differ: /me/loans is paged, the per-library endpoint returns a bare array.
func listLoans(t *testing.T, args map[string]any) ([]searched, listLoansResult, bool) {
	t.Helper()

	var got []searched
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, searched{Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/api/v1/me/loans") {
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":"l1","loaned_to":"Sam"}],"total":4}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"l1","loaned_to":"Sam"}]}`))
	}))
	defer srv.Close()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0-dev"}, nil)
	AddListLoans(mcpServer, api.New(srv.URL, "lbrm_pat_test"))

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := mcpServer.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0-dev"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_loans", Arguments: args})
	if err != nil {
		t.Fatalf("calling list_loans: %v", err)
	}
	var out listLoansResult
	if !res.IsError {
		if err := json.Unmarshal(mustJSON(t, res.StructuredContent), &out); err != nil {
			t.Fatalf("decoding the result: %v", err)
		}
	}
	return got, out, res.IsError
}

func TestLoansSpanEveryLibraryByDefault(t *testing.T) {
	// A library used to be required, so "what is out on loan?" meant listing
	// the libraries and asking each one.
	got, res, isErr := listLoans(t, map[string]any{})
	if isErr {
		t.Fatal("listing loans without a library failed; it is the common case")
	}
	if len(got) != 1 || got[0].Path != "/api/v1/me/loans" {
		t.Fatalf("requested %+v, want one call to /api/v1/me/loans", got)
	}
	if res.Total != 4 {
		t.Errorf("reported %d loans, want the server's 4", res.Total)
	}
}

func TestLoansCanAskForOverdue(t *testing.T) {
	// The question the tool exists to answer, and it could not be asked before.
	got, _, _ := listLoans(t, map[string]any{"overdue": true})
	if got[0].get("overdue") != "true" {
		t.Errorf("sent overdue=%q, want true", got[0].get("overdue"))
	}
}

func TestLoansNarrowToOneLibraryWithoutChangingEndpoint(t *testing.T) {
	got, _, _ := listLoans(t, map[string]any{"library_id": "lib-1"})
	if got[0].Path != "/api/v1/me/loans" {
		t.Errorf("used %s; a library is a filter now, not a path", got[0].Path)
	}
	if got[0].get("lib") != "lib-1" {
		t.Errorf("sent lib=%q, want lib-1", got[0].get("lib"))
	}
}

func TestOneBooksHistoryGoesThroughItsLibrary(t *testing.T) {
	// /me/loans has no book_id filter, so this path has to stay.
	got, _, isErr := listLoans(t, map[string]any{"library_id": "lib-1", "book_id": "b1"})
	if isErr {
		t.Fatal("asking for one book's history failed")
	}
	if got[0].Path != "/api/v1/libraries/lib-1/loans" {
		t.Errorf("requested %s, want the per-library endpoint", got[0].Path)
	}
	if got[0].get("book_id") != "b1" {
		t.Errorf("sent book_id=%q, want b1", got[0].get("book_id"))
	}
}

func TestBookHistoryWithoutALibraryIsRefused(t *testing.T) {
	// Dropping book_id silently would answer a question about one book with
	// every loan in the collection.
	got, _, isErr := listLoans(t, map[string]any{"book_id": "b1"})
	if !isErr {
		t.Fatalf("accepted book_id with no library and requested %+v", got)
	}
	if len(got) != 0 {
		t.Errorf("made %d requests before refusing", len(got))
	}
}
