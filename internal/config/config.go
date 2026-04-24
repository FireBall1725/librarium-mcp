// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

// Package config resolves the MCP server's runtime configuration from env
// vars and a small on-disk state file. Keeps the three-layer token
// resolution ("env → persisted file → first-run generate") in one place so
// cmd/mcp and any future integration tests share the same logic.
package config

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

// MCPTokenPrefix identifies server-side MCP tokens (distinct from Librarium
// API PATs which use `lbrm_pat_`).
const MCPTokenPrefix = "lbrm_mcp_"

const mcpTokenRandomLen = 43

// Config is the fully-resolved runtime shape, ready to hand to the server
// wiring.
type Config struct {
	// LibrariumURL is the base URL of the Librarium API this MCP server
	// talks to. No trailing slash.
	LibrariumURL string

	// LibrariumToken is the `lbrm_pat_*` credential used on outbound API
	// calls. Minted by the user in the Librarium web UI.
	LibrariumToken string

	// Listen is the address the MCP server binds to (e.g. ":8090").
	Listen string

	// MCPToken is the bearer credential incoming MCP clients must present.
	// Resolved from LIBRARIUM_MCP_TOKEN env → /data/mcp-token file →
	// first-run generation (in which case FirstRun is true and the caller
	// is expected to print the reveal banner).
	MCPToken string

	// TokenFilePath is where a generated token is stored. Only populated
	// when we generated one on this boot; callers use it in the banner so
	// users know where to go if they lose it.
	TokenFilePath string

	// FirstRun indicates the MCP token was freshly generated this boot
	// and the user needs to see it at least once.
	FirstRun bool
}

// Load resolves config from the environment. Env var names match the
// librarium-mcp.md plan document.
func Load() (*Config, error) {
	url := strings.TrimRight(strings.TrimSpace(os.Getenv("LIBRARIUM_API_URL")), "/")
	if url == "" {
		return nil, errors.New("LIBRARIUM_API_URL is required")
	}
	pat := strings.TrimSpace(os.Getenv("LIBRARIUM_ACCESS_TOKEN"))
	if pat == "" {
		return nil, errors.New("LIBRARIUM_ACCESS_TOKEN is required (mint one in the Librarium web UI under Profile → API tokens)")
	}

	listen := strings.TrimSpace(os.Getenv("LIBRARIUM_MCP_LISTEN"))
	if listen == "" {
		listen = ":8090"
	}

	// Token file path is fixed for now. If people want to relocate it
	// they can bind-mount /data to wherever and everything still works.
	dataDir := strings.TrimSpace(os.Getenv("LIBRARIUM_MCP_DATA_DIR"))
	if dataDir == "" {
		dataDir = "/data"
	}
	tokenFile := filepath.Join(dataDir, "mcp-token")

	mcpTok, firstRun, err := resolveMCPToken(tokenFile)
	if err != nil {
		return nil, err
	}

	return &Config{
		LibrariumURL:   url,
		LibrariumToken: pat,
		Listen:         listen,
		MCPToken:       mcpTok,
		TokenFilePath:  tokenFile,
		FirstRun:       firstRun,
	}, nil
}

// resolveMCPToken implements the three-tier fallback described in the plan.
// Returns (token, firstRun, err). firstRun is true only when we generated
// and persisted a fresh token this boot.
func resolveMCPToken(tokenFile string) (string, bool, error) {
	// 1. Env var overrides everything. Declarative setups (k8s, compose
	//    with secrets) land here.
	if env := strings.TrimSpace(os.Getenv("LIBRARIUM_MCP_TOKEN")); env != "" {
		return env, false, nil
	}

	// 2. Persisted file from a previous boot.
	if raw, err := os.ReadFile(tokenFile); err == nil {
		if v := strings.TrimSpace(string(raw)); v != "" {
			return v, false, nil
		}
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("reading token file %q: %w", tokenFile, err)
	}

	// 3. First run — generate, persist, and flag for the banner.
	body, err := randomBase62(mcpTokenRandomLen)
	if err != nil {
		return "", false, fmt.Errorf("generating MCP token: %w", err)
	}
	token := MCPTokenPrefix + body

	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		return "", false, fmt.Errorf("creating token dir: %w", err)
	}
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		return "", false, fmt.Errorf("writing token file %q: %w", tokenFile, err)
	}
	return token, true, nil
}

// randomBase62 returns a cryptographically-random string of length n using
// the base62 alphabet. Mirrors the librarium-api generator so both sides
// produce tokens with identical shape.
func randomBase62(n int) (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	max := big.NewInt(int64(len(alphabet)))
	buf := make([]byte, n)
	for i := range n {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[idx.Int64()]
	}
	return string(buf), nil
}
