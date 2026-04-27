// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

// Package api is a minimal HTTP client for the Librarium public API.
// Scoped to just what MCP tools call — this is intentionally not a
// generated client, since the MCP server touches only ~10 endpoints and
// the hand-written wrapper is easier to evolve alongside tool needs.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the thin Librarium HTTP client. Every outbound call carries the
// user's lbrm_pat_ bearer; 4xx/5xx responses are surfaced as Error so the
// MCP tool layer can map them to human-readable MCP errors.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Error is the shape of a non-2xx response. Status is the HTTP code, Body
// is the raw response body (capped to reasonable length upstream). Tools
// can switch on Status to produce friendlier messages (e.g. 404 → "not in
// your library").
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("librarium api: %d %s", e.Status, e.Body)
	}
	return fmt.Sprintf("librarium api: %d", e.Status)
}

// envelope mirrors the api's response shape for successful calls.
type envelope[T any] struct {
	Data T `json:"data"`
}

// Get decodes a GET response's `data` field into out.
func Get[T any](ctx context.Context, c *Client, path string) (T, error) {
	var zero T
	body, err := c.doRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return zero, err
	}
	var env envelope[T]
	if err := json.Unmarshal(body, &env); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", path, err)
	}
	return env.Data, nil
}

// Post sends a JSON body and decodes the `data` field from the response.
func Post[Req, Resp any](ctx context.Context, c *Client, path string, req Req) (Resp, error) {
	var zero Resp
	buf, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encoding %s body: %w", path, err)
	}
	respBody, err := c.doRaw(ctx, http.MethodPost, path, buf)
	if err != nil {
		return zero, err
	}
	var env envelope[Resp]
	if err := json.Unmarshal(respBody, &env); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", path, err)
	}
	return env.Data, nil
}

// Put mirrors Post for PUT requests.
func Put[Req, Resp any](ctx context.Context, c *Client, path string, req Req) (Resp, error) {
	var zero Resp
	buf, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encoding %s body: %w", path, err)
	}
	respBody, err := c.doRaw(ctx, http.MethodPut, path, buf)
	if err != nil {
		return zero, err
	}
	var env envelope[Resp]
	if err := json.Unmarshal(respBody, &env); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", path, err)
	}
	return env.Data, nil
}

// Patch mirrors Post for PATCH requests.
func Patch[Req, Resp any](ctx context.Context, c *Client, path string, req Req) (Resp, error) {
	var zero Resp
	buf, err := json.Marshal(req)
	if err != nil {
		return zero, fmt.Errorf("encoding %s body: %w", path, err)
	}
	respBody, err := c.doRaw(ctx, http.MethodPatch, path, buf)
	if err != nil {
		return zero, err
	}
	var env envelope[Resp]
	if err := json.Unmarshal(respBody, &env); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", path, err)
	}
	return env.Data, nil
}

// Delete sends a DELETE and ignores the response body. Returns an Error
// for non-2xx so callers can distinguish "deleted" from "didn't exist".
func Delete(ctx context.Context, c *Client, path string) error {
	_, err := c.doRaw(ctx, http.MethodDelete, path, nil)
	return err
}

// doRaw is the shared HTTP execute path. Returns the raw response body for
// the generic decoders above to unmarshal.
func (c *Client) doRaw(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	return nil, &Error{Status: resp.StatusCode, Body: string(raw)}
}
