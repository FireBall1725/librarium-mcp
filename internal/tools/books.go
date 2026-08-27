// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Subtitle     string           `json:"subtitle,omitempty"`
	Description  string           `json:"description,omitempty"`
	MediaType    string           `json:"media_type,omitempty"`
	Publisher    string           `json:"publisher,omitempty"`
	PublishYear  int              `json:"publish_year,omitempty"`
	Language     string           `json:"language,omitempty"`
	Contributors []contributorRef `json:"contributors,omitempty"`
	Tags         []namedRef       `json:"tags,omitempty"`
	Genres       []namedRef       `json:"genres,omitempty"`
	Series       []seriesRef      `json:"series,omitempty"`
	Libraries    []namedRef       `json:"libraries,omitempty"`
	ReadStatus   string           `json:"read_status,omitempty"`
	CoverURL     string           `json:"cover_url,omitempty"`
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
			out[i] = Library(l)
		}
		return nil, listLibrariesResult{Libraries: out}, nil
	})
}

// ─── search_books ────────────────────────────────────────────────────────────

type searchBooksArgs struct {
	Query     string `json:"query,omitempty" jsonschema:"free-text against titles and contributor names; omit it to browse by the filters alone"`
	Author    string `json:"author,omitempty" jsonschema:"an author name; resolved to that person and filtered by identity, so it will not also match books with their name in the title"`
	LibraryID string `json:"library_id,omitempty" jsonschema:"optional; when omitted, searches every library the user can see"`
	Tag       string `json:"tag,omitempty" jsonschema:"a single tag name"`
	Genre     string `json:"genre,omitempty" jsonschema:"a single genre name"`
	MediaType string `json:"media_type,omitempty" jsonschema:"a media type name, for example manga, novel, graphic_novel"`
	// The wire values, not the words a rail shows. read_status is what the
	// server calls it and what the API expects.
	ReadStatus string `json:"read_status,omitempty" jsonschema:"one of read, reading, unread, did_not_finish"`
	Include    string `json:"include,omitempty" jsonschema:"what counts as the user's books: shelf (the default, books they have), wishlist, suggested, gap, or any"`
	Limit      int    `json:"limit,omitempty" jsonschema:"max 100, default 25"`
}

type searchBooksResult struct {
	Books []BookSummary `json:"books"`
	// Total is how many matched, which is usually more than were returned.
	Total int `json:"total"`
	// Note explains a result that would otherwise read as "you own nothing by
	// them": an author the catalogue has never heard of matches no books, and
	// that is worth saying rather than returning an empty list.
	Note string `json:"note,omitempty"`
}

type apiPagedBooks struct {
	Items []apiBook `json:"items"`
	Total int       `json:"total"`
}

type apiBook struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	Subtitle     string           `json:"subtitle"`
	Description  string           `json:"description"`
	MediaType    string           `json:"media_type"`
	Publisher    string           `json:"publisher"`
	PublishYear  int              `json:"publish_year"`
	Language     string           `json:"language"`
	Contributors []apiContributor `json:"contributors"`
	Tags         []apiNamed       `json:"tags"`
	Genres       []apiNamed       `json:"genres"`
	Series       []apiSeries      `json:"series"`
	Libraries    []apiNamed       `json:"libraries"`
	UserReadSt   string           `json:"user_read_status"`
	CoverURL     string           `json:"cover_url"`
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

// AddSearchBooks wires the search_books tool.
//
// One request to /me/books rather than one per library. The old shape fetched
// the library list and then searched each library in turn, which was wrong in
// three ways beyond the request count: results came back grouped by library
// instead of ranked together, the per-library budget was an equal split so a
// collection of 1,400 books and one of 1 were each given half the limit, and
// the only thing it could filter on was free text.
func AddSearchBooks(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "search_books",
		Description: "Search the user's books across every library they can read. " +
			"Combine free text with filters: `author` names a person and filters by " +
			"identity rather than by name match, and `tag`, `genre`, `media_type` and " +
			"`read_status` narrow further. Searches books the user has; pass `include` " +
			"to reach the wishlist or suggestions. Use get_book with a returned id for " +
			"full details.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args searchBooksArgs) (*mcp.CallToolResult, searchBooksResult, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 25
		}
		if limit > 100 {
			limit = 100
		}

		q := url.Values{}
		q.Set("per_page", fmt.Sprintf("%d", limit))
		if args.Query != "" {
			q.Set("q", args.Query)
		}
		if args.LibraryID != "" {
			q.Set("lib", args.LibraryID)
		}
		if args.Tag != "" {
			q.Set("tag", args.Tag)
		}
		if args.Genre != "" {
			q.Set("genre", args.Genre)
		}
		if args.MediaType != "" {
			q.Set("type", args.MediaType)
		}
		if args.ReadStatus != "" {
			q.Set("status", args.ReadStatus)
		}

		// Books the user has, unless they asked otherwise. Without this the
		// server applies no ownership filter and suggestions and wishlist
		// entries arrive alongside the shelf, so "do I have this?" answers yes
		// about a book nobody owns.
		switch args.Include {
		case "", "shelf":
			q.Set("own", "shelf")
		case "any":
			// No ownership parameter at all, which is how the API says "every
			// state". Not the empty string: that reads as an empty filter.
		default:
			q.Set("own", args.Include)
		}

		var note string
		if args.Author != "" {
			id, name, err := resolveContributor(ctx, client, args.Author)
			if err != nil {
				return nil, searchBooksResult{}, err
			}
			if id == "" {
				// Filtering by nothing would silently widen the search to the
				// whole collection, which reads as "here is everything they
				// wrote" when the truth is nobody by that name is in it.
				return nil, searchBooksResult{
					Note: fmt.Sprintf("No contributor matching %q is in this collection, so no books were searched for them.", args.Author),
				}, nil
			}
			q.Set("contributor", id)
			if !strings.EqualFold(name, args.Author) {
				note = fmt.Sprintf("Matched the author %q.", name)
			}
		}

		paged, err := api.Get[apiPagedBooks](ctx, client, "/api/v1/me/books?"+q.Encode())
		if err != nil {
			return nil, searchBooksResult{}, err
		}
		return nil, searchBooksResult{
			Books: projectBooks(paged.Items, nil),
			Total: paged.Total,
			Note:  note,
		}, nil
	})
}

// resolveContributor turns a name into the person it names.
//
// Returns an empty id when nobody matches, which the caller has to treat as
// "no books" rather than "no filter": a dropped filter widens the search to
// everything, and everything is a much worse answer than nothing.
func resolveContributor(ctx context.Context, client *api.Client, name string) (id, matched string, err error) {
	hits, err := api.Get[[]apiNamed](ctx, client,
		"/api/v1/contributors?q="+url.QueryEscape(name))
	if err != nil {
		return "", "", fmt.Errorf("looking up %q: %w", name, err)
	}
	if len(hits) == 0 {
		return "", "", nil
	}
	// An exact name wins over a longer one that merely contains it, so asking
	// for "Tite Kubo" does not land on "Tite Kubo Illustration Works".
	for _, h := range hits {
		if strings.EqualFold(h.Name, name) {
			return h.ID, h.Name, nil
		}
	}
	return hits[0].ID, hits[0].Name, nil
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
		contribs[i] = contributorRef(c)
	}
	tags := make([]namedRef, len(b.Tags))
	for i, t := range b.Tags {
		tags[i] = namedRef(t)
	}
	genres := make([]namedRef, len(b.Genres))
	for i, g := range b.Genres {
		genres[i] = namedRef(g)
	}
	libs := make([]namedRef, len(b.Libraries))
	for i, l := range b.Libraries {
		libs[i] = namedRef(l)
	}
	series := make([]seriesRef, len(b.Series))
	for i, s := range b.Series {
		series[i] = seriesRef(s)
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
