// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// call drives a tool the way a real client does, through the MCP server, and
// reports every HTTP request the tool made.
//
// Going through the server rather than calling the closure directly is the
// point: it exercises argument binding, so a rename that breaks the schema
// shows up here instead of at runtime.
func call(t *testing.T, wire func(*mcp.Server, *api.Client), tool string, args map[string]any) []recorded {
	t.Helper()

	var got []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = append(got, recorded{Method: r.Method, Path: r.URL.Path, Body: body})

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"book_id":"b1","read_status":"read","rating":8,
			"is_favorite":true,"review":"r","notes":"n","wants":false,"inherited":false}`))
	}))
	defer srv.Close()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0-dev"}, nil)
	wire(mcpServer, api.New(srv.URL, "lbrm_pat_test"))

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

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("calling %s: %v", tool, err)
	}
	if res.IsError {
		t.Fatalf("%s returned an error result: %+v", tool, res.Content)
	}
	return got
}

type recorded struct {
	Method string
	Path   string
	Body   map[string]any
}

// TestEachToolIsOneRequest is the whole point of moving reading state to the
// work. Each of these used to be three calls: list the book's editions to guess
// which printing the caller meant, GET the current row, then PUT it back merged.
// The read-then-write also raced itself, so a rating set on a phone between the
// GET and the PUT was silently overwritten.
func TestEachToolIsOneRequest(t *testing.T) {
	cases := []struct {
		name string
		wire func(*mcp.Server, *api.Client)
		tool string
		args map[string]any
	}{
		{"read status", AddSetReadStatus, "set_read_status",
			map[string]any{"book_id": "b1", "status": "read"}},
		{"rating", AddSetRating, "set_rating",
			map[string]any{"book_id": "b1", "rating": 8}},
		{"review", AddWriteReview, "write_review",
			map[string]any{"book_id": "b1", "review": "good"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := call(t, c.wire, c.tool, c.args)
			if len(got) != 1 {
				t.Fatalf("made %d requests, want 1: %+v", len(got), got)
			}
			if got[0].Method != http.MethodPut {
				t.Errorf("method = %s, want PUT", got[0].Method)
			}
			if got[0].Path != "/api/v1/books/b1/me" {
				t.Errorf("path = %s, want the work-keyed route", got[0].Path)
			}
		})
	}
}

// TestToolsSendOnlyTheFieldsTheyOwn covers the reason the endpoint is a partial
// update. A tool that sets a rating must not send a review, or it would blank
// one typed on another device.
func TestToolsSendOnlyTheFieldsTheyOwn(t *testing.T) {
	cases := []struct {
		name    string
		wire    func(*mcp.Server, *api.Client)
		tool    string
		args    map[string]any
		want    map[string]any
		absent  []string
		comment string
	}{
		{
			name: "read status touches nothing else",
			wire: AddSetReadStatus, tool: "set_read_status",
			args:   map[string]any{"book_id": "b1", "status": "reading"},
			want:   map[string]any{"read_status": "reading"},
			absent: []string{"rating", "review", "notes", "is_favorite", "clear_rating"},
		},
		{
			name: "a rating is sent as a rating",
			wire: AddSetRating, tool: "set_rating",
			args:   map[string]any{"book_id": "b1", "rating": 6},
			want:   map[string]any{"rating": float64(6)},
			absent: []string{"read_status", "review", "notes", "is_favorite", "clear_rating"},
		},
		{
			// The tool's contract is "null clears", and the server reads an
			// omitted rating as "no change", so nil has to become an explicit
			// clear or clearing a rating would silently do nothing.
			name: "a null rating clears explicitly",
			wire: AddSetRating, tool: "set_rating",
			args:   map[string]any{"book_id": "b1", "rating": nil},
			want:   map[string]any{"clear_rating": true},
			absent: []string{"rating", "read_status", "review", "notes", "is_favorite"},
		},
		{
			name: "a review does not disturb a rating",
			wire: AddWriteReview, tool: "write_review",
			args:   map[string]any{"book_id": "b1", "review": "good"},
			want:   map[string]any{"review": "good"},
			absent: []string{"rating", "clear_rating", "read_status", "notes", "is_favorite"},
		},
		{
			// An explicit empty string clears, which is different from omitting
			// the field, and the pointer is what carries that difference.
			name: "an empty string clears rather than being dropped",
			wire: AddWriteReview, tool: "write_review",
			args:   map[string]any{"book_id": "b1", "notes": ""},
			want:   map[string]any{"notes": ""},
			absent: []string{"review", "rating", "read_status", "is_favorite"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := call(t, c.wire, c.tool, c.args)
			if len(got) != 1 {
				t.Fatalf("made %d requests, want 1", len(got))
			}
			body := got[0].Body
			for k, v := range c.want {
				if body[k] != v {
					t.Errorf("body[%q] = %#v, want %#v", k, body[k], v)
				}
			}
			for _, k := range c.absent {
				if _, present := body[k]; present {
					t.Errorf("body carried %q = %#v, which this tool does not own", k, body[k])
				}
			}
		})
	}
}
