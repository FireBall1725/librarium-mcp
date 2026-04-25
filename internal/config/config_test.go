// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupCleanEnv blanks every config-relevant env var so individual
// tests can opt-in to exactly what they want without leaking state.
// Uses t.Setenv so cleanup happens automatically when the test ends.
func setupCleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"LIBRARIUM_API_URL",
		"LIBRARIUM_ACCESS_TOKEN",
		"LIBRARIUM_MCP_LISTEN",
		"LIBRARIUM_MCP_DATA_DIR",
		"LIBRARIUM_MCP_TOKEN",
	} {
		t.Setenv(k, "")
	}
}

func TestLoad_RequiresURL(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_ACCESS_TOKEN", "lbrm_pat_test")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no LIBRARIUM_API_URL: want error, got nil")
	}
	if !strings.Contains(err.Error(), "LIBRARIUM_API_URL") {
		t.Errorf("error %q should name the missing variable", err)
	}
}

func TestLoad_RequiresAccessToken(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_API_URL", "http://localhost:8080")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no LIBRARIUM_ACCESS_TOKEN: want error, got nil")
	}
	if !strings.Contains(err.Error(), "LIBRARIUM_ACCESS_TOKEN") {
		t.Errorf("error %q should name the missing variable", err)
	}
}

func TestLoad_StripsTrailingSlash(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_API_URL", "http://localhost:8080/")
	t.Setenv("LIBRARIUM_ACCESS_TOKEN", "lbrm_pat_test")
	t.Setenv("LIBRARIUM_MCP_DATA_DIR", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LibrariumURL != "http://localhost:8080" {
		t.Errorf("LibrariumURL = %q, want %q", cfg.LibrariumURL, "http://localhost:8080")
	}
}

func TestLoad_DefaultsListen(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_API_URL", "http://localhost:8080")
	t.Setenv("LIBRARIUM_ACCESS_TOKEN", "lbrm_pat_test")
	t.Setenv("LIBRARIUM_MCP_DATA_DIR", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8090" {
		t.Errorf("Listen = %q, want default %q", cfg.Listen, ":8090")
	}
}

func TestLoad_HonoursListenOverride(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_API_URL", "http://localhost:8080")
	t.Setenv("LIBRARIUM_ACCESS_TOKEN", "lbrm_pat_test")
	t.Setenv("LIBRARIUM_MCP_LISTEN", "0.0.0.0:9999")
	t.Setenv("LIBRARIUM_MCP_DATA_DIR", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "0.0.0.0:9999" {
		t.Errorf("Listen = %q, want %q", cfg.Listen, "0.0.0.0:9999")
	}
}

// TestResolveMCPToken_EnvWins covers the declarative-setup path
// (k8s/compose with a secret-mounted env var) — the env value is
// returned verbatim and no file is written.
func TestResolveMCPToken_EnvWins(t *testing.T) {
	setupCleanEnv(t)
	t.Setenv("LIBRARIUM_MCP_TOKEN", "lbrm_mcp_envprovided")
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "mcp-token")

	tok, firstRun, err := resolveMCPToken(tokenFile)
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if tok != "lbrm_mcp_envprovided" {
		t.Errorf("token = %q, want env value", tok)
	}
	if firstRun {
		t.Error("firstRun = true; env-provided tokens are never first-run")
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Error("token file was written even though env var was set")
	}
}

// TestResolveMCPToken_ReadsExistingFile is the warm-restart path —
// previous boot persisted a token, this boot reads it back.
func TestResolveMCPToken_ReadsExistingFile(t *testing.T) {
	setupCleanEnv(t)
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "mcp-token")
	persisted := "lbrm_mcp_persistedFromPriorBoot"
	if err := os.WriteFile(tokenFile, []byte(persisted+"\n"), 0o600); err != nil {
		t.Fatalf("seeding token file: %v", err)
	}

	tok, firstRun, err := resolveMCPToken(tokenFile)
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if tok != persisted {
		t.Errorf("token = %q, want %q (trimmed)", tok, persisted)
	}
	if firstRun {
		t.Error("firstRun = true; reading an existing file is not first-run")
	}
}

// TestResolveMCPToken_GeneratesAndPersists exercises the cold-boot
// path: no env, no file, must generate, persist, and flag firstRun.
// This is the most security-relevant code path — a regression that
// returned a static or guessable token would compromise every
// freshly-deployed instance.
func TestResolveMCPToken_GeneratesAndPersists(t *testing.T) {
	setupCleanEnv(t)
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "mcp-token")

	tok, firstRun, err := resolveMCPToken(tokenFile)
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if !firstRun {
		t.Error("firstRun = false; cold-boot generation should set the flag")
	}
	if !strings.HasPrefix(tok, MCPTokenPrefix) {
		t.Errorf("token %q lacks expected prefix %q", tok, MCPTokenPrefix)
	}
	if got := len(tok); got != len(MCPTokenPrefix)+mcpTokenRandomLen {
		t.Errorf("token length = %d, want %d", got, len(MCPTokenPrefix)+mcpTokenRandomLen)
	}

	// File must be persisted with restrictive perms — anyone who can
	// read the host filesystem inherits MCP control otherwise.
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perms = %o, want 0600", perm)
	}

	// Re-resolve — should now read the file we just wrote, not
	// regenerate a fresh token.
	again, firstRunAgain, err := resolveMCPToken(tokenFile)
	if err != nil {
		t.Fatalf("resolveMCPToken (second call): %v", err)
	}
	if again != tok {
		t.Errorf("second resolve returned different token %q, want %q", again, tok)
	}
	if firstRunAgain {
		t.Error("firstRun should be false on subsequent resolves")
	}
}

// TestRandomBase62 is a smoke test on the entropy source — we can't
// assert "is random" but we can pin length and alphabet.
func TestRandomBase62(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	got, err := randomBase62(43)
	if err != nil {
		t.Fatalf("randomBase62: %v", err)
	}
	if len(got) != 43 {
		t.Errorf("len = %d, want 43", len(got))
	}
	for i, c := range got {
		if !strings.ContainsRune(alphabet, c) {
			t.Errorf("byte %d (%q) not in base62 alphabet", i, c)
		}
	}
}
