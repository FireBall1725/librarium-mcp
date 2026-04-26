// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddSuggestionsRecent wires librarium://suggestions/recent — the most
// recent AI-generated suggestions for the authenticated user (buy / read-
// next candidates with their lifecycle status). Mirrors what the
// get_recent_suggestions tool returns, available without a tool call.
func AddSuggestionsRecent(srv *mcp.Server, client *api.Client) {
	const uri = "librarium://suggestions/recent"
	srv.AddResource(&mcp.Resource{
		URI:         uri,
		Name:        "Recent suggestions",
		Title:       "Recent AI suggestions",
		Description: "The current user's most recent AI-generated suggestions, with their lifecycle status (interested / dismissed / added to library).",
		MIMEType:    "application/json",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/me/suggestions")
		if err != nil {
			return nil, apiError(uri, err)
		}
		return passthrough(uri, raw), nil
	})
}
