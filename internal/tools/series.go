// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Series, which is the shape most questions about a collection actually have.
//
// "Am I missing anything from Bleach" is a question about a run, and until now
// the only way to ask it here was to search books and count by hand. It is also
// the question the catalogue only recently learned to answer: a volume nobody
// holds is a book row now, so a gap is something the server can name rather
// than something a reader infers from a hole in the numbering.

// ─── Wire shapes ────────────────────────────────────────────────────────────

type apiSeriesRow struct {
	ID         string   `json:"id"`
	LibraryID  string   `json:"library_id"`
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	TotalCount *int     `json:"total_count"`
	BookCount  int      `json:"book_count"`
	ReadCount  int      `json:"read_count"`
	Rating     *int     `json:"rating"`
	RatedBooks int      `json:"rated_books"`
	Genres     []string `json:"genres"`
}

type apiSeriesPage struct {
	Items []apiSeriesRow `json:"items"`
	Total int            `json:"total"`
}

// SeriesSummary is what a run looks like in a conversation.
//
// Missing is spelled out rather than left as two numbers to subtract, because
// it is the thing people ask for and an LLM doing the arithmetic itself is an
// LLM that can get it wrong.
type SeriesSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status,omitempty"`
	Owned   int    `json:"owned"`
	Total   int    `json:"total,omitempty"`
	Missing int    `json:"missing,omitempty"`
	Read    int    `json:"read,omitempty"`
	// Rating is out of five with halves, converted from the stored ten-point
	// scale, because a 8 means nothing to a reader who counts stars.
	Rating float64  `json:"rating,omitempty"`
	Genres []string `json:"genres,omitempty"`
}

type apiSeriesEntry struct {
	Position    float64  `json:"position"`
	PositionEnd *float64 `json:"position_end"`
	BookID      string   `json:"book_id"`
	Title       string   `json:"title"`
	Held        bool     `json:"held"`
	ReadStatus  string   `json:"user_read_status"`
}

// VolumeSummary is one volume of a run, held or not.
type VolumeSummary struct {
	Position   string `json:"position"`
	BookID     string `json:"book_id"`
	Title      string `json:"title"`
	Held       bool   `json:"held"`
	ReadStatus string `json:"read_status,omitempty"`
}

// ─── list_series ────────────────────────────────────────────────────────────

type listSeriesArgs struct {
	Query     string `json:"query,omitempty" jsonschema:"free-text against series names; omit it to list them all"`
	LibraryID string `json:"library_id,omitempty" jsonschema:"optional; when omitted, every library the user can see"`
	Genre     string `json:"genre,omitempty" jsonschema:"a single genre name"`
	MediaType string `json:"media_type,omitempty" jsonschema:"a media type name, for example manga, novel, graphic_novel"`
	Status    string `json:"status,omitempty" jsonschema:"publication status: ongoing, completed, hiatus or cancelled"`
	// The one people actually want, and the reason this tool exists.
	Incomplete bool   `json:"incomplete,omitempty" jsonschema:"only runs with volumes the user does not have"`
	Sort       string `json:"sort,omitempty" jsonschema:"name (default), volumes, missing, read or rating"`
	Limit      int    `json:"limit,omitempty" jsonschema:"max 100, default 25"`
}

type listSeriesResult struct {
	Series []SeriesSummary `json:"series"`
	Total  int             `json:"total"`
	Note   string          `json:"note,omitempty"`
}

func AddListSeries(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_series",
		Description: "List the reading runs in the user's collection, with how many volumes " +
			"they have of each and how many are missing. Pass `incomplete` to see only runs " +
			"with gaps, which is the usual reason to ask. Filter by `genre`, `media_type` or " +
			"`status`, and sort by `missing` to put the biggest gaps first. Use get_series " +
			"with a returned id to see which volumes are missing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listSeriesArgs) (*mcp.CallToolResult, listSeriesResult, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 25
		}
		if limit > 100 {
			limit = 100
		}

		q := url.Values{}
		if args.Query != "" {
			q.Set("q", args.Query)
		}
		if args.LibraryID != "" {
			q.Set("lib", args.LibraryID)
		}
		if args.Genre != "" {
			q.Set("genre", args.Genre)
		}
		if args.MediaType != "" {
			q.Set("type", args.MediaType)
		}
		if args.Status != "" {
			q.Set("status", args.Status)
		}
		if args.Sort != "" && args.Sort != "name" {
			q.Set("sort", args.Sort)
			// Descending for every sort but name: nobody asks for the run with
			// the fewest volumes missing.
			q.Set("dir", "desc")
		}
		// One volume per row rather than the full strip. The index builds a
		// preview of up to sixty covers per series and none of them can be
		// rendered in a conversation.
		q.Set("volumes", "1")

		path := "/api/v1/me/series/index"
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		res, err := api.Get[apiSeriesPage](ctx, client, path)
		if err != nil {
			return nil, listSeriesResult{}, fmt.Errorf("listing series: %w", err)
		}

		out := listSeriesResult{Series: make([]SeriesSummary, 0, limit)}
		for _, s := range res.Items {
			missing := 0
			total := 0
			if s.TotalCount != nil {
				total = *s.TotalCount
				if d := total - s.BookCount; d > 0 {
					missing = d
				}
			}
			if args.Incomplete && missing == 0 {
				continue
			}
			out.Total++
			if len(out.Series) >= limit {
				continue
			}
			sum := SeriesSummary{
				ID: s.ID, Name: s.Name, Status: s.Status,
				Owned: s.BookCount, Total: total, Missing: missing,
				Read: s.ReadCount, Genres: s.Genres,
			}
			if s.Rating != nil {
				sum.Rating = float64(*s.Rating) / 2
			}
			out.Series = append(out.Series, sum)
		}

		if args.Incomplete && out.Total == 0 {
			// Worth saying rather than returning an empty list, which reads as
			// a failed lookup rather than as good news.
			out.Note = "Nothing is missing: every run matching that filter is complete."
		}
		return nil, out, nil
	})
}

// ─── get_series ─────────────────────────────────────────────────────────────

type getSeriesArgs struct {
	SeriesID string `json:"series_id,omitempty" jsonschema:"the id from list_series; give this or name"`
	Name     string `json:"name,omitempty" jsonschema:"the run's name, if the id is not to hand; the closest match is used"`
	Only     string `json:"only,omitempty" jsonschema:"all (default), missing to list only volumes the user does not have, or held"`
}

type getSeriesResult struct {
	Series  SeriesSummary   `json:"series"`
	Volumes []VolumeSummary `json:"volumes"`
	Note    string          `json:"note,omitempty"`
}

func AddGetSeries(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "get_series",
		Description: "The volumes of one run, in reading order, saying which the user has " +
			"and which are missing. Accepts a series id or a name. Pass `only: missing` to " +
			"answer \"what am I missing\" directly.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getSeriesArgs) (*mcp.CallToolResult, getSeriesResult, error) {
		if args.SeriesID == "" && args.Name == "" {
			return nil, getSeriesResult{}, fmt.Errorf("give a series_id or a name")
		}

		var found *apiSeriesRow
		q := url.Values{}
		q.Set("volumes", "1")
		if args.Name != "" {
			q.Set("q", args.Name)
		}
		index, err := api.Get[apiSeriesPage](ctx, client, "/api/v1/me/series/index?"+q.Encode())
		if err != nil {
			return nil, getSeriesResult{}, fmt.Errorf("finding the series: %w", err)
		}
		for i := range index.Items {
			if args.SeriesID != "" {
				if index.Items[i].ID == args.SeriesID {
					found = &index.Items[i]
					break
				}
				continue
			}
			// Closest by name: an exact match wins, otherwise the shortest name
			// containing what was asked for, so "bleach" does not land on
			// "Bleach Official Character Book".
			if strings.EqualFold(index.Items[i].Name, args.Name) {
				found = &index.Items[i]
				break
			}
			if found == nil || len(index.Items[i].Name) < len(found.Name) {
				found = &index.Items[i]
			}
		}
		if found == nil {
			return nil, getSeriesResult{
				Note: "No run in the collection matches that.",
			}, nil
		}

		entries, err := api.Get[[]apiSeriesEntry](ctx, client,
			fmt.Sprintf("/api/v1/libraries/%s/series/%s/books", found.LibraryID, found.ID))
		if err != nil {
			return nil, getSeriesResult{}, fmt.Errorf("listing the volumes: %w", err)
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Position < entries[j].Position })

		out := getSeriesResult{Volumes: make([]VolumeSummary, 0, len(entries))}
		missing := 0
		for _, e := range entries {
			if !e.Held {
				missing++
			}
			switch args.Only {
			case "missing":
				if e.Held {
					continue
				}
			case "held":
				if !e.Held {
					continue
				}
			}
			vol := VolumeSummary{
				Position: formatPosition(e.Position, e.PositionEnd),
				BookID:   e.BookID,
				Title:    e.Title,
				Held:     e.Held,
			}
			// Only for a volume somebody has. The server says "unread" about a
			// book nobody owns, which is true and reads as a to-do rather than
			// as a gap.
			if e.Held {
				vol.ReadStatus = e.ReadStatus
			}
			out.Volumes = append(out.Volumes, vol)
		}

		total := 0
		if found.TotalCount != nil {
			total = *found.TotalCount
		}
		out.Series = SeriesSummary{
			ID: found.ID, Name: found.Name, Status: found.Status,
			Owned: found.BookCount, Total: total, Missing: missing,
			Read: found.ReadCount, Genres: found.Genres,
		}
		if found.Rating != nil {
			out.Series.Rating = float64(*found.Rating) / 2
		}
		if args.Only == "missing" && missing == 0 {
			out.Note = "Nothing is missing from this run."
		}
		return nil, out, nil
	})
}

// formatPosition writes 3 rather than 3.0, keeps 4.5, and writes a span as
// 1-3 for an omnibus, which occupies the positions of everything it contains
// rather than one of its own.
func formatPosition(pos float64, end *float64) string {
	one := func(v float64) string {
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return strings.TrimRight(fmt.Sprintf("%.1f", v), "0")
	}
	if end != nil && *end > pos {
		return one(pos) + "-" + one(*end)
	}
	return one(pos)
}
