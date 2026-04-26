// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddBookDetail wires librarium://book/{id} — a single book by id. Hits
// the library-agnostic endpoint so the LLM can drill into a book without
// also tracking which library it belongs to.
func AddBookDetail(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://book/{id}"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Book",
		Title:       "Book detail",
		Description: "Full details for a single book by id: title, contributors, tags, genres, series, libraries it belongs to, and the user's read status. Library-agnostic — no library id required.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/books/"+params["id"])
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}
