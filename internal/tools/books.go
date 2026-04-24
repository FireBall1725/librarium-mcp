// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"net/url"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ─── Wire shapes ────────────────────────────────────────────────────────────

// Library is the trimmed shape we hand to the LLM. The full api row carries
// server-context fields that don't make sense on a single-upstream MCP
// server, so we deliberately project only what's useful for conversation.
type Library struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsPublic    bool   `json:"is_public,omitempty"`
}

// BookSummary is the shape returned by search_books — compact so 25+ results
// fit in the LLM's context without drowning it in fields the user didn't ask
// for. Full metadata is fetched via get_book when the user drills in.
type BookSummary struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle,omitempty"`
	Authors  []string `json:"authors,omitempty"`
	Library  string   `json:"library,omitempty"` // library name for cross-library searches
}

// Book is the full shape handed back by get_book. Includes edition metadata
// and interaction state so the LLM can answer "did I read it" / "what did I
// rate it" without chaining another tool call.
type Book struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Subtitle      string           `json:"subtitle,omitempty"`
	Description   string           `json:"description,omitempty"`
	MediaType     string           `json:"media_type,omitempty"`
	Publisher     string           `json:"publisher,omitempty"`
	PublishYear   int              `json:"publish_year,omitempty"`
	Language      string           `json:"language,omitempty"`
	Contributors  []contributorRef `json:"contributors,omitempty"`
	Tags          []namedRef       `json:"tags,omitempty"`
	Genres        []namedRef       `json:"genres,omitempty"`
	Series        []seriesRef      `json:"series,omitempty"`
	Libraries     []namedRef       `json:"libraries,omitempty"`
	ReadStatus    string           `json:"read_status,omitempty"`
	CoverURL      string           `json:"cover_url,omitempty"`
}

type contributorRef struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
type namedRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}
type seriesRef struct {
	Name     string  `json:"name"`
	Position float64 `json:"position,omitempty"`
}

// ─── list_libraries ──────────────────────────────────────────────────────────

type listLibrariesArgs struct{}

type listLibrariesResult struct {
	Libraries []Library `json:"libraries"`
}

// apiLibrary is the raw api row shape — richer than what we hand the LLM.
type apiLibrary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

// AddListLibraries wires the list_libraries tool onto the server. Reads
// every library the authenticated user can see and returns a compact list
// so the LLM can reference them by name or id in subsequent tool calls.
func AddListLibraries(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_libraries",
		Description: "List every library the current user can access on this Librarium instance. Returns id + name + description for each. Use these ids when calling search_books for a specific library.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listLibrariesArgs) (*mcp.CallToolResult, listLibrariesResult, error) {
		libs, err := api.Get[[]apiLibrary](ctx, client, "/api/v1/libraries")
		if err != nil {
			return nil, listLibrariesResult{}, err
		}
		out := make([]Library, len(libs))
		for i, l := range libs {
			out[i] = Library{
				ID:          l.ID,
				Name:        l.Name,
				Description: l.Description,
				IsPublic:    l.IsPublic,
			}
		}
		return nil, listLibrariesResult{Libraries: out}, nil
	})
}

// ─── search_books ────────────────────────────────────────────────────────────

type searchBooksArgs struct {
	Query     string `json:"query" jsonschema:"free-text search against titles, authors, and subtitles"`
	LibraryID string `json:"library_id,omitempty" jsonschema:"optional; when omitted, searches every library the user can see"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max 100, default 25"`
}

type searchBooksResult struct {
	Books []BookSummary `json:"books"`
	Total int           `json:"total"`
}

type apiPagedBooks struct {
	Items []apiBook `json:"items"`
	Total int       `json:"total"`
}

type apiBook struct {
	ID           string               `json:"id"`
	Title        string               `json:"title"`
	Subtitle     string               `json:"subtitle"`
	Description  string               `json:"description"`
	MediaType    string               `json:"media_type"`
	Publisher    string               `json:"publisher"`
	PublishYear  int                  `json:"publish_year"`
	Language     string               `json:"language"`
	Contributors []apiContributor     `json:"contributors"`
	Tags         []apiNamed           `json:"tags"`
	Genres       []apiNamed           `json:"genres"`
	Series       []apiSeries          `json:"series"`
	Libraries    []apiNamed           `json:"libraries"`
	UserReadSt   string               `json:"user_read_status"`
	CoverURL     string               `json:"cover_url"`
}
type apiContributor struct {
	Name string `json:"name"`
	Role string `json:"role"`
}
type apiNamed struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type apiSeries struct {
	Name     string  `json:"series_name"`
	Position float64 `json:"position"`
}

// AddSearchBooks wires the search_books tool. Hits a single library when
// library_id is provided; otherwise fans out across every library the user
// can see and merges the results. Per-library limit is capped so a cross-
// library search doesn't blow out the LLM's context.
func AddSearchBooks(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_books",
		Description: "Search the user's libraries by title, subtitle, or author. Returns up to `limit` compact summaries (default 25, max 100). Scope to one library with `library_id` or omit to search everywhere. Use get_book with a returned id for full details.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchBooksArgs) (*mcp.CallToolResult, searchBooksResult, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 25
		}
		if limit > 100 {
			limit = 100
		}

		if args.LibraryID != "" {
			paged, err := searchOneLibrary(ctx, client, args.LibraryID, args.Query, limit)
			if err != nil {
				return nil, searchBooksResult{}, err
			}
			return nil, searchBooksResult{Books: projectBooks(paged.Items, nil), Total: paged.Total}, nil
		}

		// No library specified — fan out. Fetch the libraries first so we
		// can stamp each book's "library name" for the LLM to disambiguate.
		libs, err := api.Get[[]apiLibrary](ctx, client, "/api/v1/libraries")
		if err != nil {
			return nil, searchBooksResult{}, err
		}
		// Budget the per-library limit so the aggregate total can still fit
		// under the user's `limit`. Cheap equal split; good enough for v1.
		perLibrary := limit / max(len(libs), 1)
		if perLibrary < 5 {
			perLibrary = 5 // don't make any one library useless
		}

		var all []BookSummary
		var total int
		for _, lib := range libs {
			paged, err := searchOneLibrary(ctx, client, lib.ID, args.Query, perLibrary)
			if err != nil {
				// Skip libraries the user can't read rather than aborting
				// the whole search; surface as empty.
				continue
			}
			total += paged.Total
			libName := lib.Name
			all = append(all, projectBooks(paged.Items, &libName)...)
			if len(all) >= limit {
				all = all[:limit]
				break
			}
		}
		return nil, searchBooksResult{Books: all, Total: total}, nil
	})
}

func searchOneLibrary(ctx context.Context, client *api.Client, libraryID, query string, limit int) (*apiPagedBooks, error) {
	q := url.Values{}
	q.Set("per_page", fmt.Sprintf("%d", limit))
	if query != "" {
		q.Set("q", query)
	}
	path := fmt.Sprintf("/api/v1/libraries/%s/books?%s", libraryID, q.Encode())
	paged, err := api.Get[apiPagedBooks](ctx, client, path)
	if err != nil {
		return nil, err
	}
	return &paged, nil
}

func projectBooks(in []apiBook, libraryName *string) []BookSummary {
	out := make([]BookSummary, len(in))
	for i, b := range in {
		authors := authorsFromContributors(b.Contributors)
		bs := BookSummary{
			ID:       b.ID,
			Title:    b.Title,
			Subtitle: b.Subtitle,
			Authors:  authors,
		}
		if libraryName != nil {
			bs.Library = *libraryName
		}
		out[i] = bs
	}
	return out
}

func authorsFromContributors(cs []apiContributor) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		if c.Role == "" || c.Role == "author" {
			out = append(out, c.Name)
		}
	}
	return out
}

// ─── get_book ────────────────────────────────────────────────────────────────

type getBookArgs struct {
	BookID string `json:"book_id" jsonschema:"uuid from list_libraries/search_books"`
}

// AddGetBook wires the get_book tool. Returns the full book record plus
// edition + user-interaction state so the LLM can answer detail questions
// without additional round-trips.
func AddGetBook(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_book",
		Description: "Get the full details for a single book by id: title, subtitle, contributors (authors, illustrators, narrators), tags, genres, series, the libraries it lives in, and your read status. Use ids returned by search_books or list_libraries.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getBookArgs) (*mcp.CallToolResult, Book, error) {
		if args.BookID == "" {
			return nil, Book{}, fmt.Errorf("book_id is required")
		}
		b, err := api.Get[apiBook](ctx, client, "/api/v1/books/"+args.BookID)
		if err != nil {
			return nil, Book{}, err
		}
		return nil, projectBook(b), nil
	})
}

func projectBook(b apiBook) Book {
	contribs := make([]contributorRef, len(b.Contributors))
	for i, c := range b.Contributors {
		contribs[i] = contributorRef{Name: c.Name, Role: c.Role}
	}
	tags := make([]namedRef, len(b.Tags))
	for i, t := range b.Tags {
		tags[i] = namedRef{ID: t.ID, Name: t.Name}
	}
	genres := make([]namedRef, len(b.Genres))
	for i, g := range b.Genres {
		genres[i] = namedRef{ID: g.ID, Name: g.Name}
	}
	libs := make([]namedRef, len(b.Libraries))
	for i, l := range b.Libraries {
		libs[i] = namedRef{ID: l.ID, Name: l.Name}
	}
	series := make([]seriesRef, len(b.Series))
	for i, s := range b.Series {
		series[i] = seriesRef{Name: s.Name, Position: s.Position}
	}
	return Book{
		ID:           b.ID,
		Title:        b.Title,
		Subtitle:     b.Subtitle,
		Description:  b.Description,
		MediaType:    b.MediaType,
		Publisher:    b.Publisher,
		PublishYear:  b.PublishYear,
		Language:     b.Language,
		Contributors: contribs,
		Tags:         tags,
		Genres:       genres,
		Series:       series,
		Libraries:    libs,
		ReadStatus:   b.UserReadSt,
		CoverURL:     b.CoverURL,
	}
}
