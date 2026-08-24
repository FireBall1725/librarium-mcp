// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// add_book_by_isbn — the write counterpart to lookup_isbn + search_books.
// Looks up the ISBN's metadata, resolves the caller-supplied media type
// name to the server's uuid, creates the book + first edition in the
// target library, and kicks off a metadata + cover enrichment job so the
// book has data filled in asynchronously.
//
// Mirrors what the iOS scan flow's Add sheet does, just with LLM-supplied
// inputs instead of sheet inputs. Takes media type and format by *name*
// ("novel" / "paperback") so the LLM doesn't have to juggle UUIDs.

type addBookByISBNArgs struct {
	ISBN      string `json:"isbn" jsonschema:"ISBN-10 or ISBN-13 of the book to add"`
	LibraryID string `json:"library_id" jsonschema:"library to add the book to; from list_libraries"`
	MediaType string `json:"media_type" jsonschema:"one of: novel, manga, comic — matches a media type on the server"`
	Format    string `json:"format" jsonschema:"one of: paperback, hardcover, ebook, audiobook"`
}

type addBookByISBNResult struct {
	BookID  string `json:"book_id"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// apiMediaType is the media-types endpoint row.
type apiMediaType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// createBookRequest mirrors the api's CreateBookRequest shape. Keep naming
// in sync with internal/api/handlers/books.go's bookRequestBody.
type createBookRequest struct {
	Title       string              `json:"title"`
	Subtitle    string              `json:"subtitle,omitempty"`
	MediaTypeID string              `json:"media_type_id"`
	Description string              `json:"description,omitempty"`
	Edition     *createEditionInput `json:"edition,omitempty"`
}

type createEditionInput struct {
	Format      string `json:"format"`
	Language    string `json:"language,omitempty"`
	Publisher   string `json:"publisher,omitempty"`
	PublishDate string `json:"publish_date,omitempty"`
	ISBN10      string `json:"isbn_10,omitempty"`
	ISBN13      string `json:"isbn_13,omitempty"`
	Description string `json:"description,omitempty"`
	PageCount   int    `json:"page_count,omitempty"`
	IsPrimary   bool   `json:"is_primary"`
}

// apiCreateBookResponse is what POST /libraries/{id}/books returns.
type apiCreateBookResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// AddAddBookByISBN wires the add_book_by_isbn tool.
func AddAddBookByISBN(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "add_book_by_isbn",
		Description: "Add a book to one of the user's libraries. Looks up the ISBN through the configured metadata providers, resolves the media type name (novel/manga/comic) to the server's uuid, creates the book + first edition with the chosen format (paperback/hardcover/ebook/audiobook), and kicks off a metadata + cover enrichment job so fields fill in asynchronously. Returns the new book_id for follow-up tool calls (set_read_status, etc.).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args addBookByISBNArgs) (*mcp.CallToolResult, addBookByISBNResult, error) {
		if args.ISBN == "" || args.LibraryID == "" || args.MediaType == "" || args.Format == "" {
			return nil, addBookByISBNResult{}, fmt.Errorf("isbn, library_id, media_type, and format are all required")
		}

		// Resolve media type name → uuid. Server exposes this list to any
		// authed caller; it's global, not per-library.
		mediaTypes, err := api.Get[[]apiMediaType](ctx, client, "/api/v1/media-types")
		if err != nil {
			return nil, addBookByISBNResult{}, fmt.Errorf("loading media types: %w", err)
		}
		mediaTypeID := ""
		target := strings.ToLower(strings.TrimSpace(args.MediaType))
		for _, mt := range mediaTypes {
			if strings.ToLower(mt.Name) == target || strings.ToLower(mt.DisplayName) == target {
				mediaTypeID = mt.ID
				break
			}
		}
		if mediaTypeID == "" {
			names := make([]string, 0, len(mediaTypes))
			for _, mt := range mediaTypes {
				names = append(names, mt.Name)
			}
			return nil, addBookByISBNResult{}, fmt.Errorf("no media type matches %q; available: %s", args.MediaType, strings.Join(names, ", "))
		}

		// Merged ISBN lookup populates the book + edition fields.
		merged, err := api.Get[apiMergedLookup](ctx, client, "/api/v1/lookup/isbn/"+args.ISBN+"/merged")
		if err != nil {
			return nil, addBookByISBNResult{}, fmt.Errorf("isbn lookup: %w", err)
		}
		lookup := flattenMerged(merged)
		if lookup.Title == "" {
			return nil, addBookByISBNResult{}, fmt.Errorf("no metadata found for ISBN %s", args.ISBN)
		}

		req := createBookRequest{
			Title:       lookup.Title,
			Subtitle:    lookup.Subtitle,
			MediaTypeID: mediaTypeID,
			Description: lookup.Description,
			Edition: &createEditionInput{
				Format:      strings.ToLower(strings.TrimSpace(args.Format)),
				Language:    lookup.Language,
				Publisher:   lookup.Publisher,
				PublishDate: lookup.PublishDate,
				ISBN10:      lookup.ISBN10,
				ISBN13:      lookup.ISBN13,
				Description: lookup.Description,
				PageCount:   lookup.PageCount,
				IsPrimary:   true,
			},
		}
		created, err := api.Post[createBookRequest, apiCreateBookResponse](
			ctx, client,
			"/api/v1/libraries/"+args.LibraryID+"/books",
			req,
		)
		if err != nil {
			return nil, addBookByISBNResult{}, fmt.Errorf("creating book: %w", err)
		}

		// The edition listing that used to happen here is gone. It existed only
		// to hand an edition_id to the interaction tools, and reading state is
		// keyed to the work now, so the follow-up call needs the book_id this
		// response already carries.

		// Kick off a server-side enrichment so cover + missing metadata fill
		// in asynchronously. Fire-and-forget; a failure here isn't fatal.
		if created.ID != "" {
			_, _ = api.Post[struct{}, struct{}](ctx, client, "/api/v1/books/"+created.ID+"/enrich", struct{}{})
		}

		return nil, addBookByISBNResult{
			BookID:  created.ID,
			Title:   created.Title,
			Message: "Added. Metadata + cover enrichment queued; refetch with get_book in a few seconds to see enriched fields.",
		}, nil
	})
}
