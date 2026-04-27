// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

// Package resources exposes the MCP resource surface of the Librarium MCP
// server. Resources are read-only catalog views the LLM (or end-user via
// /resource UI) can pull on demand without burning a tool-call slot.
//
// Each resource is a thin pass-through: validate the requested URI, hit
// the public Librarium API via the shared client, hand the unwrapped
// `data` field back as JSON. v1 doesn't project — the api shape becomes
// the resource shape, same coupling we already accept for tools.
//
// v1 surface:
//   - librarium://libraries
//   - librarium://library/{id}
//   - librarium://library/{id}/books
//   - librarium://library/{id}/series
//   - librarium://library/{id}/loans
//   - librarium://library/{lib}/series/{sid}
//   - librarium://book/{id}
//   - librarium://book/{id}/loans
//   - librarium://suggestions/recent
//   - librarium://stats
package resources

import (
	"errors"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll wires every resource this package owns onto the given MCP
// server. Mirrors tools.RegisterAll so cmd/mcp/main.go stays a single line.
func RegisterAll(srv *mcp.Server, client *api.Client) {
	AddLibrariesList(srv, client)
	AddLibraryDetail(srv, client)
	AddLibraryBooks(srv, client)
	AddLibrarySeries(srv, client)
	AddSeriesDetail(srv, client)
	AddBookDetail(srv, client)
	AddSuggestionsRecent(srv, client)
	AddStats(srv, client)
	AddLibraryLoans(srv, client)
	AddBookLoans(srv, client)
}

// apiError translates a librarium api.Error into the MCP error shape.
// Upstream 404 collapses to ResourceNotFoundError so the client UI treats
// it as a missing resource, not a generic failure. Everything else passes
// through untouched.
func apiError(uri string, err error) error {
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return mcp.ResourceNotFoundError(uri)
	}
	return err
}
