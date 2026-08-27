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

// The bodies below are the shapes the server actually sends, taken off a
// running instance rather than written from the handler source. Two runs: one
// complete, one a volume short, which is the difference the tools exist to
// report.
const seriesIndexBody = `{"data":{"total":2,"items":[
  {"id":"s-ab","library_id":"lib-1","name":"Absolute Boyfriend","status":"ongoing",
   "book_count":6,"total_count":7,"read_count":2,"rating":8,"rated_books":3,
   "genres":["Romance","Science Fiction"]},
  {"id":"s-ah","library_id":"lib-1","name":"After Hours","status":"completed",
   "book_count":3,"total_count":3,"read_count":0,"rating":null,"rated_books":0,
   "genres":["Girls' Love"]}
]}}`

const seriesBooksBody = `{"data":[
  {"position":1,"position_end":3,"book_id":"b1","title":"Absolute Boyfriend 3-in-1","held":true,"user_read_status":"read"},
  {"position":4,"position_end":null,"book_id":"b4","title":"Absolute Boyfriend #4","held":true,"user_read_status":"reading"},
  {"position":7,"position_end":null,"book_id":"b7","title":"Absolute Boyfriend #7","held":false,"user_read_status":"unread"}
]}`

// callSeries drives one of the series tools through a real MCP session and
// reports what it asked the API for.
func callSeries(t *testing.T, tool string, args map[string]any, out any) []searched {
	t.Helper()

	var got []searched
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, searched{Path: r.URL.Path, Query: r.URL.Query()})
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/books") {
			_, _ = w.Write([]byte(seriesBooksBody))
			return
		}
		_, _ = w.Write([]byte(seriesIndexBody))
	}))
	defer srv.Close()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0-dev"}, nil)
	client := api.New(srv.URL, "lbrm_pat_test")
	AddListSeries(mcpServer, client)
	AddGetSeries(mcpServer, client)

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := mcpServer.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	defer serverSession.Close()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0-dev"}, nil).
		Connect(ctx, ct, nil)
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
	if err := json.Unmarshal(mustJSON(t, res.StructuredContent), out); err != nil {
		t.Fatalf("decoding the result: %v", err)
	}
	return got
}

func TestListSeriesReportsWhatIsMissing(t *testing.T) {
	// The number people ask for. Leaving it as two counts to subtract puts the
	// arithmetic somewhere it can be got wrong.
	var res listSeriesResult
	callSeries(t, "list_series", map[string]any{}, &res)

	if len(res.Series) != 2 {
		t.Fatalf("returned %d runs, want 2", len(res.Series))
	}
	if res.Series[0].Missing != 1 {
		t.Errorf("Absolute Boyfriend is missing %d, want 1", res.Series[0].Missing)
	}
	if res.Series[1].Missing != 0 {
		t.Errorf("After Hours is complete but reports %d missing", res.Series[1].Missing)
	}
}

func TestListSeriesConvertsTheRatingToStars(t *testing.T) {
	// Stored in whole points where two points are one star. Handing an 8 to a
	// reader who counts stars out of five is a wrong number, not a raw one.
	var res listSeriesResult
	callSeries(t, "list_series", map[string]any{}, &res)

	if res.Series[0].Rating != 4 {
		t.Errorf("reported %v stars for a stored 8, want 4", res.Series[0].Rating)
	}
	if res.Series[1].Rating != 0 {
		t.Errorf("an unrated run reported %v, want it left out", res.Series[1].Rating)
	}
}

func TestIncompleteDropsTheCompleteRuns(t *testing.T) {
	var res listSeriesResult
	callSeries(t, "list_series", map[string]any{"incomplete": true}, &res)

	if len(res.Series) != 1 || res.Series[0].Name != "Absolute Boyfriend" {
		t.Fatalf("returned %+v, want only the run with a gap", res.Series)
	}
	// Total counts what matched the filter, not what the server sent, or
	// "1 of 2" reads as a truncated list rather than as one incomplete run.
	if res.Total != 1 {
		t.Errorf("reported %d matches, want 1", res.Total)
	}
}

func TestListSeriesDoesNotAskForTheCoverStrip(t *testing.T) {
	// The index builds up to sixty preview rows per series for a page that
	// draws covers. None of them can be rendered in a conversation.
	got := callSeries(t, "list_series", map[string]any{}, &listSeriesResult{})
	if got[0].get("volumes") != "1" {
		t.Errorf("asked for volumes=%q, want 1", got[0].get("volumes"))
	}
}

func TestGetSeriesListsEveryVolumeInOrder(t *testing.T) {
	var res getSeriesResult
	got := callSeries(t, "get_series", map[string]any{"series_id": "s-ab"}, &res)

	if len(got) != 2 || !strings.HasSuffix(got[1].Path, "/series/s-ab/books") {
		t.Fatalf("requested %+v, want the index then that run's books", got)
	}
	if len(res.Volumes) != 3 {
		t.Fatalf("returned %d volumes, want 3", len(res.Volumes))
	}
	// An omnibus covers the positions of what is inside it rather than
	// occupying one of its own.
	if res.Volumes[0].Position != "1-3" {
		t.Errorf("the 3-in-1 sits at %q, want 1-3", res.Volumes[0].Position)
	}
	if res.Series.Missing != 1 {
		t.Errorf("reported %d missing, want 1", res.Series.Missing)
	}
}

func TestOnlyMissingReturnsJustTheGaps(t *testing.T) {
	var res getSeriesResult
	callSeries(t, "get_series", map[string]any{"series_id": "s-ab", "only": "missing"}, &res)

	if len(res.Volumes) != 1 || res.Volumes[0].Position != "7" {
		t.Fatalf("returned %+v, want volume 7 alone", res.Volumes)
	}
	// The count comes off the whole run even when the list is filtered, so
	// "1 of 7" stays true rather than becoming "1 of 1".
	if res.Series.Owned != 6 || res.Series.Total != 7 {
		t.Errorf("reported %d of %d, want 6 of 7", res.Series.Owned, res.Series.Total)
	}
}

func TestGetSeriesByNameSearchesRatherThanScanning(t *testing.T) {
	var res getSeriesResult
	got := callSeries(t, "get_series", map[string]any{"name": "after hours"}, &res)

	if got[0].get("q") != "after hours" {
		t.Errorf("sent q=%q, want the name", got[0].get("q"))
	}
	// Exact wins over the other row the stub returns, whatever order it is in.
	if res.Series.Name != "After Hours" {
		t.Errorf("matched %q, want After Hours", res.Series.Name)
	}
}

func TestGetSeriesNeedsSomethingToLookUp(t *testing.T) {
	// Neither argument means the tool would list every run and pick one, which
	// is worse than saying so.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(seriesIndexBody))
	}))
	defer srv.Close()

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0-dev"}, nil)
	AddGetSeries(mcpServer, api.New(srv.URL, "lbrm_pat_test"))

	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := mcpServer.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("connecting server: %v", err)
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.0-dev"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connecting client: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_series", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("calling get_series: %v", err)
	}
	if !res.IsError {
		t.Error("accepted a call with no series_id and no name")
	}
}
