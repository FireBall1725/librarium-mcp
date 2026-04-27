// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddLibraryLoans wires librarium://library/{id}/loans — every active
// (not yet returned) loan in a single library. Pass-through projection
// like the rest of the resources; the LLM gets the api shape.
func AddLibraryLoans(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://library/{id}/loans"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Library loans",
		Title:       "Active loans in a library",
		Description: "Every active (not yet returned) loan in a single library by id. For per-book history use librarium://book/{id}/loans, or call the list_loans tool with include_returned=true.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/libraries/"+params["id"]+"/loans")
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}

// AddBookLoans wires librarium://book/{id}/loans — every loan ever
// recorded for a specific book (active and returned). Useful for
// answering "who has borrowed this before". Library-scoped reads happen
// via the library_books junction implicitly: each loan row carries its
// own library_id so the consumer can disambiguate when a book lives in
// multiple libraries.
//
// Implementation note: the library_id query path requires a library
// context. We can't take just a book id and ask the api for "all loans
// across libraries" — there's no such endpoint. So we list libraries,
// fan out per-library with `book_id=` filter, and merge. Cheap because
// most users have 1–3 libraries.
func AddBookLoans(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://book/{id}/loans"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Book loans",
		Title:       "Loan history for a book",
		Description: "Every loan recorded for a single book by id, across every library the user can see. Includes active and returned loans.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		bookID := params["id"]
		libs, err := api.Get[[]apiLibraryRow](ctx, client, "/api/v1/libraries")
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		merged := []json.RawMessage{}
		for _, lib := range libs {
			path := "/api/v1/libraries/" + lib.ID + "/loans?include_returned=true&book_id=" + bookID
			rows, err := api.Get[[]json.RawMessage](ctx, client, path)
			if err != nil {
				continue // skip libraries the user can't read
			}
			merged = append(merged, rows...)
		}
		body, err := json.Marshal(merged)
		if err != nil {
			return nil, err
		}
		return passthrough(req.Params.URI, body), nil
	})
}

// apiLibraryRow is the trimmed library shape we need for the fan-out in
// book/{id}/loans. Local to this file so we don't tangle with shapes used
// by libraries.go which projects different fields.
type apiLibraryRow struct {
	ID string `json:"id"`
}
