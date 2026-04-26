// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddSeriesDetail wires librarium://library/{lib}/series/{sid} — a single
// series with its volumes/books, status, demographic, and arcs (when the
// series has been split). The library id is part of the URI because the
// api scopes series reads to a library context.
func AddSeriesDetail(srv *mcp.Server, client *api.Client) {
	const tmpl = "librarium://library/{lib}/series/{sid}"
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: tmpl,
		Name:        "Series",
		Title:       "Series detail",
		Description: "A single series with status, total volumes, and arcs (if any). The library id scopes the read; pair with librarium://library/{id}/series to find a series id.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		params, ok := matchURI(req.Params.URI, tmpl)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		path := "/api/v1/libraries/" + params["lib"] + "/series/" + params["sid"]
		raw, err := api.Get[json.RawMessage](ctx, client, path)
		if err != nil {
			return nil, apiError(req.Params.URI, err)
		}
		return passthrough(req.Params.URI, raw), nil
	})
}
