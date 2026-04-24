// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// lookup_isbn — provider-merged ISBN lookup identical to what the iOS scan
// flow consumes. Useful on its own for "what's the metadata for this ISBN"
// questions, and as a pre-step for add_book_by_isbn so the LLM can confirm
// the book identity before adding.

type lookupISBNArgs struct {
	ISBN string `json:"isbn" jsonschema:"ISBN-10 or ISBN-13 with or without hyphens"`
}

// LookupResult is the flat, LLM-friendly projection of the merged response.
// Provider attribution and per-field alternatives are dropped; callers get
// one best-guess value per field the way the iOS scan card shows it.
type LookupResult struct {
	Title       string   `json:"title,omitempty"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	PublishDate string   `json:"publish_date,omitempty"`
	ISBN10      string   `json:"isbn_10,omitempty"`
	ISBN13      string   `json:"isbn_13,omitempty"`
	Description string   `json:"description,omitempty"`
	CoverURL    string   `json:"cover_url,omitempty"`
	Language    string   `json:"language,omitempty"`
	PageCount   int      `json:"page_count,omitempty"`
	Categories  []string `json:"categories,omitempty"`
}

// fieldResult mirrors providers.FieldResult — the api returns each string
// field as an object with a `value` plus provider-source attribution. A nil
// pointer means no provider returned that field.
type fieldResult struct {
	Value string `json:"value"`
}

// coverOption mirrors providers.CoverOption. MCP tools only need the URL.
type coverOption struct {
	CoverURL string `json:"cover_url"`
}

// apiMergedLookup matches the providers.MergedBookResult shape on the wire.
// Every scalar field is a pointer because the api omits the key when no
// provider returned a value; nil → empty projection.
type apiMergedLookup struct {
	Title       *fieldResult  `json:"title"`
	Subtitle    *fieldResult  `json:"subtitle"`
	Authors     *fieldResult  `json:"authors"`      // comma-joined
	Description *fieldResult  `json:"description"`
	Publisher   *fieldResult  `json:"publisher"`
	PublishDate *fieldResult  `json:"publish_date"`
	Language    *fieldResult  `json:"language"`
	ISBN10      *fieldResult  `json:"isbn_10"`
	ISBN13      *fieldResult  `json:"isbn_13"`
	PageCount   *fieldResult  `json:"page_count"`  // string-cast int
	Categories  []string      `json:"categories"`
	Covers      []coverOption `json:"covers"`
}

// fieldValue pulls the .Value out of a FieldResult pointer safely.
func fieldValue(f *fieldResult) string {
	if f == nil {
		return ""
	}
	return f.Value
}

// flattenMerged projects the wire shape into the flat form every tool
// that consumes the merged lookup wants: strings instead of sourced
// wrappers, a single cover URL, authors as a slice, page count as int.
func flattenMerged(m apiMergedLookup) LookupResult {
	out := LookupResult{
		Title:       fieldValue(m.Title),
		Subtitle:    fieldValue(m.Subtitle),
		Publisher:   fieldValue(m.Publisher),
		PublishDate: fieldValue(m.PublishDate),
		ISBN10:      fieldValue(m.ISBN10),
		ISBN13:      fieldValue(m.ISBN13),
		Description: fieldValue(m.Description),
		Language:    fieldValue(m.Language),
		Categories:  m.Categories,
	}
	if authors := fieldValue(m.Authors); authors != "" {
		parts := strings.Split(authors, ", ")
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out.Authors = append(out.Authors, p)
			}
		}
	}
	if pc := fieldValue(m.PageCount); pc != "" {
		if n, err := strconv.Atoi(pc); err == nil {
			out.PageCount = n
		}
	}
	if len(m.Covers) > 0 {
		out.CoverURL = m.Covers[0].CoverURL
	}
	return out
}

// AddLookupISBN wires the lookup_isbn tool. Hits the api's merged provider
// lookup, which fans out across every configured metadata provider (Google
// Books, Open Library, Hardcover, etc.) and returns one best-guess row.
func AddLookupISBN(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "lookup_isbn",
		Description: "Look up book metadata for an ISBN-10 or ISBN-13. Merges results from every metadata provider the server has configured (Google Books, Open Library, etc.) into one best-guess record: title, authors, publisher, description, cover URL, and both ISBN forms. Not scoped to any library — this is a catalog lookup, not an ownership check. Use search_books or get_book for what's actually in your library.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args lookupISBNArgs) (*mcp.CallToolResult, LookupResult, error) {
		if args.ISBN == "" {
			return nil, LookupResult{}, fmt.Errorf("isbn is required")
		}
		path := "/api/v1/lookup/isbn/" + url.PathEscape(args.ISBN) + "/merged"
		merged, err := api.Get[apiMergedLookup](ctx, client, path)
		if err != nil {
			return nil, LookupResult{}, err
		}
		return nil, flattenMerged(merged), nil
	})
}
