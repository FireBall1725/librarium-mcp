// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

// Command mcp is the Librarium MCP server entrypoint. It presents a
// Model Context Protocol surface over streamable HTTP, translating each
// tool call into an authenticated request against the Librarium public
// API. No special backend access; the MCP server is just another client.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/fireball1725/librarium-mcp/internal/config"
	"github.com/fireball1725/librarium-mcp/internal/resources"
	"github.com/fireball1725/librarium-mcp/internal/tools"
	"github.com/fireball1725/librarium-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("startup config invalid", "error", err)
		os.Exit(1)
	}

	logger.Info("librarium-mcp", "version", version.BuildVersion, "listen", cfg.Listen, "upstream", cfg.LibrariumURL)
	if cfg.FirstRun {
		printFirstRunBanner(cfg.MCPToken, cfg.TokenFilePath)
	}

	// A single Librarium API client is shared across all incoming MCP
	// sessions — there's only one identity on this server (the minting
	// user's PAT), so there's nothing session-specific to isolate.
	client := api.New(cfg.LibrariumURL, cfg.LibrariumToken)

	// One MCP server instance, reused across every incoming session. All
	// sessions operate as the same Librarium user so there's no per-request
	// configuration we'd need to vary.
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "librarium",
		Version: version.BuildVersion,
	}, nil)
	tools.RegisterAll(mcpServer, client)
	resources.RegisterAll(mcpServer, client)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("/mcp", requireBearer(cfg.MCPToken, mcpHandler))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM so container stops cleanly.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		logger.Info("http server listening", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server crashed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// ─── handlers ────────────────────────────────────────────────────────────────

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"version": version.BuildVersion,
	})
}

// ─── middleware ──────────────────────────────────────────────────────────────

// requireBearer gates MCP traffic behind the server's inbound auth token.
// Constant-time compare avoids leaking token length via response timing.
func requireBearer(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(got) == 0 || !constantTimeEq(got, token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "missing or invalid authorization header",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// constantTimeEq compares two strings in length-independent time.
func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range len(a) {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// ─── startup banner ──────────────────────────────────────────────────────────

// printFirstRunBanner dumps a highly-visible block to stdout the first time
// the server auto-generates an inbound auth token. Printed exactly once per
// install; subsequent boots read the token from the persisted file.
func printFirstRunBanner(token, tokenFile string) {
	line := strings.Repeat("═", 63)
	fmt.Println()
	fmt.Printf("╔%s╗\n", line)
	fmt.Printf("║  Librarium MCP — first run                                    ║\n")
	fmt.Printf("║                                                               ║\n")
	fmt.Printf("║  A new MCP access token has been generated:                   ║\n")
	fmt.Printf("║                                                               ║\n")
	fmt.Printf("║    %-59s║\n", token)
	fmt.Printf("║                                                               ║\n")
	fmt.Printf("║  Stored at %-51s║\n", tokenFile)
	fmt.Printf("║  Set it in your Claude Desktop config as                      ║\n")
	fmt.Printf("║    Authorization: Bearer <the token above>                    ║\n")
	fmt.Printf("║                                                               ║\n")
	fmt.Printf("║  This banner will not print again. To rotate, delete the      ║\n")
	fmt.Printf("║  token file and restart the container.                        ║\n")
	fmt.Printf("╚%s╝\n", line)
	fmt.Println()
}
