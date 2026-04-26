// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddLibrariesList wires librarium://libraries — a flat list of every
// library the authenticated user can see on this Librarium instance. The
// LLM uses these ids to drive the parameterized resources below.
func AddLibrariesList(srv *mcp.Server, client *api.Client) {
	const uri = "librarium://libraries"
	srv.AddResource(&mcp.Resource{
		URI:         uri,
		Name:        "Libraries",
		Title:       "All libraries",
		Description: "List of every library the current user can access on this Librarium instance.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/libraries")
		if err != nil {
			return nil, apiError(uri, err)
		}
		return passthrough(uri, raw), nil
	})
}

// AddLibraryDetail wires librarium://library/{id} — a single library's
// metadata (name, description, role, member count, etc.) without its book
// list. Use librarium://library/{id}/books for the contents.
func AddLibraryDetail(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://library/{id}"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Library",
		Title:       "Library detail",
		Description: "Metadata for a single library by id (no book list).",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/libraries/"+params["id"])
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}

// AddLibraryBooks wires librarium://library/{id}/books — the books in a
// single library. Returns the first page (api default ~25); use the
// search_books tool for filtered/paged access.
func AddLibraryBooks(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://library/{id}/books"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Library books",
		Title:       "Books in a library",
		Description: "First page of books in a single library by id. For filtered or larger result sets, use the search_books tool.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/libraries/"+params["id"]+"/books")
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}

// AddLibrarySeries wires librarium://library/{id}/series — the series in a
// single library. Each entry is a series header (name, position, total
// volumes); use librarium://library/{id}/series/{sid} for the volumes.
func AddLibrarySeries(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://library/{id}/series"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Library series",
		Title:       "Series in a library",
		Description: "Every series tracked in a single library by id. Each entry is a header; the per-series volumes live at librarium://library/{id}/series/{sid}.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/libraries/"+params["id"]+"/series")
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}

// passthrough wraps a raw JSON payload as a single text resource content
// block. The api envelope's `data` field has already been unwrapped by the
// generic api.Get; this just attaches the URI + mime type so the SDK can
// hand it back to the client without a second marshal.
func passthrough(uri string, raw json.RawMessage) *mcp.ReadResourceResult {
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(raw),
		}},
	}
}
