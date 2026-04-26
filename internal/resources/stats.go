// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package resources

import (
	"context"
	"encoding/json"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddStats wires librarium://stats — instance-level counters and recent
// activity for the dashboard surface (libraries, books, recent reads,
// etc.). Lets the LLM answer "how big is my collection" without iterating
// every library.
func AddStats(srv *mcp.Server, client *api.Client) {
	const uri = "librarium://stats"
	srv.AddResource(&mcp.Resource{
		URI:         uri,
		Name:        "Stats",
		Title:       "Instance stats",
		Description: "Aggregate counts (libraries, books, recent activity) across every library the current user can see. Useful for high-level overviews without iterating every library.",
		MIMEType:    "application/json",
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		raw, err := api.Get[json.RawMessage](ctx, client, "/api/v1/dashboard/stats")
		if err != nil {
			return nil, apiError(uri, err)
		}
		return passthrough(uri, raw), nil
	})
}
