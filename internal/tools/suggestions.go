// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// get_recent_suggestions — returns the caller's most recent AI suggestion
// runs along with the books each one produced. Useful when the user wants
// to ask "what did you suggest this morning" or "what was on my last list".
//
// The api's /me/suggestions endpoint returns live suggestions keyed by the
// caller, not by run, so we fan out one request per run (using the `run_id`
// query param) to stitch the book list onto each run. The endpoint
// explicitly allows run-scoped reads and returns every suggestion from
// that run regardless of status.

type getRecentSuggestionsArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"how many recent runs to include (default 5, max 25)"`
}

type SuggestionRun struct {
	ID               string         `json:"id"`
	Status           string         `json:"status"`
	TriggeredBy      string         `json:"triggered_by,omitempty"`
	ProviderType     string         `json:"provider_type,omitempty"`
	ModelID          string         `json:"model_id,omitempty"`
	SuggestionCount  int            `json:"suggestion_count"`
	StartedAt        string         `json:"started_at,omitempty"`
	FinishedAt       string         `json:"finished_at,omitempty"`
	EstimatedCostUSD float64        `json:"estimated_cost_usd,omitempty"`
	Suggestions      []SuggestionIt `json:"suggestions,omitempty"`
}

// SuggestionIt is a single book the model recommended — deliberately slim.
// Per-suggestion steering metadata and candidate reasons are dropped; the
// LLM only needs enough to answer "what did you suggest".
type SuggestionIt struct {
	BookID    string `json:"book_id,omitempty"`
	Title     string `json:"title"`
	Author    string `json:"author,omitempty"`
	ISBN      string `json:"isbn,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
	Type      string `json:"type,omitempty"`   // "buy" or "read_next"
	Status    string `json:"status,omitempty"` // new | dismissed | interested | added_to_library
}

type getRecentSuggestionsResult struct {
	Runs []SuggestionRun `json:"runs"`
}

// API shapes for decoding.

type apiSuggestionRun struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	TriggeredBy      string  `json:"triggered_by"`
	ProviderType     string  `json:"provider_type"`
	ModelID          string  `json:"model_id"`
	SuggestionCount  int     `json:"suggestion_count"`
	StartedAt        string  `json:"started_at"`
	FinishedAt       string  `json:"finished_at"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// apiSuggestion mirrors handlers.SuggestionView exactly — the api returns
// this shape from /me/suggestions.
type apiSuggestion struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	BookID    string `json:"book_id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	ISBN      string `json:"isbn"`
	Reasoning string `json:"reasoning"`
	Status    string `json:"status"`
}

// AddGetRecentSuggestions wires the tool onto the server.
func AddGetRecentSuggestions(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_recent_suggestions",
		Description: "Return the caller's most recent AI suggestion runs and the books each one produced. Each run carries its provider/model, status, cost, and an embedded list of suggestions (title, author, reasoning, type). Default 5 runs, max 25. Useful for 'what did you suggest this morning' or 'remind me of my last reading list'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args getRecentSuggestionsArgs) (*mcp.CallToolResult, getRecentSuggestionsResult, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 5
		}
		if limit > 25 {
			limit = 25
		}

		runs, err := api.Get[[]apiSuggestionRun](ctx, client, fmt.Sprintf("/api/v1/me/suggestions/runs?limit=%d", limit))
		if err != nil {
			return nil, getRecentSuggestionsResult{}, err
		}

		out := make([]SuggestionRun, len(runs))
		for i, r := range runs {
			suggs, perr := api.Get[[]apiSuggestion](ctx, client, "/api/v1/me/suggestions?run_id="+r.ID)
			var items []SuggestionIt
			if perr == nil {
				items = make([]SuggestionIt, len(suggs))
				for j, s := range suggs {
					items[j] = SuggestionIt{
						BookID:    s.BookID,
						Title:     s.Title,
						Author:    s.Author,
						ISBN:      s.ISBN,
						Reasoning: s.Reasoning,
						Type:      s.Type,
						Status:    s.Status,
					}
				}
			}
			// If the per-run fetch errored (network blip, run deleted mid-iteration),
			// leave Suggestions nil on that run rather than failing the whole tool.
			out[i] = SuggestionRun{
				ID:               r.ID,
				Status:           r.Status,
				TriggeredBy:      r.TriggeredBy,
				ProviderType:     r.ProviderType,
				ModelID:          r.ModelID,
				SuggestionCount:  r.SuggestionCount,
				StartedAt:        r.StartedAt,
				FinishedAt:       r.FinishedAt,
				EstimatedCostUSD: r.EstimatedCostUSD,
				Suggestions:      items,
			}
		}
		return nil, getRecentSuggestionsResult{Runs: out}, nil
	})
}
