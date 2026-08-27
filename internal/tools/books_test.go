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

// search drives search_books through the MCP server and reports every request
// it made, with the query string intact.
//
// Its own harness rather than the one in interactions_test.go: search reads two
// different endpoints and has to be told apart what each returns, where the
// interaction tools each hit one path and can share a single canned body.
func search(t *testing.T, args map[string]any, contributors string) ([]searched, searchBooksResult) {
	t.Helper()

	var got []searched
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, searched{Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/contributors"):
			_, _ = w.Write([]byte(contributors))
		default:
			// Wrapped in data, because respond.JSON wraps every response and
			// api.Get unwraps it. A bare body here would test a shape the
			// server never sends.
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":"b1","title":"Bleach #1",
				"contributors":[{"name":"Tite Kubo","role":"author"}]}],"total":83}}`))
		}
	}))
	defer srv.Close()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0-dev"}, nil)
	AddSearchBooks(mcpServer, api.New(srv.URL, "lbrm_pat_test"))

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

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_books", Arguments: args})
	if err != nil {
		t.Fatalf("calling search_books: %v", err)
	}
	if res.IsError {
		t.Fatalf("search_books returned an error result: %+v", res.Content)
	}

	var out searchBooksResult
	if err := json.Unmarshal(mustJSON(t, res.StructuredContent), &out); err != nil {
		t.Fatalf("decoding the result: %v", err)
	}
	return got, out
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("re-encoding the result: %v", err)
	}
	return b
}

type searched struct {
	Path  string
	Query map[string][]string
}

func (s searched) get(key string) string {
	if v := s.Query[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

const kubo = `{"data":[{"id":"c-kubo","name":"Tite Kubo"}]}`

func TestSearchIsOneRequestAcrossEveryLibrary(t *testing.T) {
	// The old shape listed the libraries and then searched each one, so the
	// request count grew with the collection and the results came back grouped
	// by library rather than ranked together.
	got, res := search(t, map[string]any{"query": "bleach"}, kubo)

	if len(got) != 1 {
		t.Fatalf("made %d requests, want 1: %+v", len(got), got)
	}
	if got[0].Path != "/api/v1/me/books" {
		t.Errorf("searched %s, want /api/v1/me/books", got[0].Path)
	}
	if got[0].get("q") != "bleach" {
		t.Errorf("sent q=%q, want bleach", got[0].get("q"))
	}
	if res.Total != 83 {
		t.Errorf("reported %d matches, want the server's 83", res.Total)
	}
}

func TestSearchAsksForBooksTheUserHas(t *testing.T) {
	// With no ownership filter the server returns every state, so suggestions
	// and wishlist entries arrive beside the shelf and "do I have this?"
	// answers yes about a book nobody owns.
	got, _ := search(t, map[string]any{"query": "bleach"}, kubo)
	if got[0].get("own") != "shelf" {
		t.Errorf("sent own=%q, want shelf", got[0].get("own"))
	}
}

func TestSearchCanReachEveryOwnershipState(t *testing.T) {
	got, _ := search(t, map[string]any{"query": "bleach", "include": "any"}, kubo)
	// Absent, not empty: an empty value reads as a filter matching nothing.
	if _, sent := got[0].Query["own"]; sent {
		t.Errorf("sent own=%q for include=any, want it omitted", got[0].get("own"))
	}
}

func TestSearchByAuthorFiltersByIdentityNotByName(t *testing.T) {
	// The whole point of the author argument: "Tite" the person is not a book
	// with Tite in its title.
	got, _ := search(t, map[string]any{"author": "Tite Kubo"}, kubo)

	if len(got) != 2 {
		t.Fatalf("made %d requests, want 2 (resolve, then search): %+v", len(got), got)
	}
	if got[0].Path != "/api/v1/contributors" {
		t.Errorf("resolved against %s, want /api/v1/contributors", got[0].Path)
	}
	if got[1].get("contributor") != "c-kubo" {
		t.Errorf("filtered by contributor=%q, want c-kubo", got[1].get("contributor"))
	}
	if got[1].get("q") != "" {
		t.Errorf("also sent q=%q; naming an author is not a text search", got[1].get("q"))
	}
}

func TestSearchSaysSoWhenTheAuthorIsUnknown(t *testing.T) {
	// Dropping the filter would widen the search to the whole collection and
	// read as "here is everything they wrote", which is a much worse answer
	// than nothing.
	got, res := search(t, map[string]any{"author": "Nobody At All"}, `{"data":[]}`)

	if len(got) != 1 {
		t.Fatalf("made %d requests, want only the failed lookup: %+v", len(got), got)
	}
	if len(res.Books) != 0 {
		t.Errorf("returned %d books for an author nobody has", len(res.Books))
	}
	if res.Note == "" {
		t.Error("returned an empty list with nothing to explain it")
	}
}

func TestSearchPrefersTheExactAuthor(t *testing.T) {
	// Both contain what was typed, and the server returns them by name, so
	// without a preference the longer one wins on sort order alone.
	both := `{"data":[{"id":"c-works","name":"Tite Kubo Illustration Works"},
	                  {"id":"c-kubo","name":"Tite Kubo"}]}`
	got, _ := search(t, map[string]any{"author": "Tite Kubo"}, both)
	if got[1].get("contributor") != "c-kubo" {
		t.Errorf("filtered by %q, want the exact match c-kubo", got[1].get("contributor"))
	}
}

func TestSearchPassesEveryFilterThrough(t *testing.T) {
	got, _ := search(t, map[string]any{
		"tag": "signed", "genre": "Manga", "media_type": "manga",
		"read_status": "unread", "library_id": "lib-1", "limit": 10,
	}, kubo)

	for key, want := range map[string]string{
		"tag": "signed", "genre": "Manga", "type": "manga",
		"status": "unread", "lib": "lib-1", "per_page": "10",
	} {
		if got[0].get(key) != want {
			t.Errorf("sent %s=%q, want %q", key, got[0].get(key), want)
		}
	}
}

func TestSearchCapsTheResultSize(t *testing.T) {
	// A cross-library search that returns everything drowns the model's
	// context, which is the reason the cap exists rather than tidiness.
	got, _ := search(t, map[string]any{"query": "a", "limit": 5000}, kubo)
	if got[0].get("per_page") != "100" {
		t.Errorf("asked for per_page=%q, want it capped at 100", got[0].get("per_page"))
	}
}
