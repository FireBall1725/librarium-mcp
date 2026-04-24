// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

// Package tools exposes the MCP tool surface of the Librarium MCP server.
// Each tool is a thin translator: validate LLM-supplied arguments, call
// the public Librarium API via the shared client, project the response
// into a shape that's compact and useful inside an LLM conversation.
//
// The catalogue in this PR is intentionally read-only: list_libraries,
// search_books, get_book. Writes land in the next PR.
package tools

import (
	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterAll wires every tool this package owns onto the given MCP server.
// Keeps the main.go wiring to one call regardless of how the tool count
// grows; new tool files just add one more Add* to this function.
func RegisterAll(srv *mcp.Server, client *api.Client) {
	AddListLibraries(srv, client)
	AddSearchBooks(srv, client)
	AddGetBook(srv, client)
}
