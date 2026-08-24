// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The three write tools here (set_read_status, set_rating, write_review) all
// operate on /books/{book_id}/me, which is one request each.
//
// They used to be three: list the book's editions to pick one, GET the current
// interaction, then PUT the whole row back merged. That existed because reading
// state was keyed to a printing, so a tool had to guess which printing the
// caller meant, and because the old endpoint was a full upsert that wiped any
// field the body omitted.
//
// Neither is true now. Reading state belongs to the work, so there is nothing
// to resolve, and the endpoint is a partial update, so a body naming one field
// leaves the rest alone. That is also the correct behaviour rather than merely
// the cheaper one: the old read-then-write raced itself, and a rating set from
// a phone between the GET and the PUT was silently overwritten.
//
// library_id and edition_id are gone from every tool below. An opinion belongs
// to neither, so accepting them would invite the caller to supply something the
// server cannot act on.

// ─── Shared shapes ──────────────────────────────────────────────────────────

// readingState mirrors readingStateBody in api/handlers/reading_state.go.
type readingState struct {
	BookID     string `json:"book_id"`
	ReadStatus string `json:"read_status"`
	Rating     *int   `json:"rating"`
	IsFavorite bool   `json:"is_favorite"`
	Review     string `json:"review"`
	Notes      string `json:"notes"`
	Wants      bool   `json:"wants"`
	// Inherited means the status came from a container the caller has read, an
	// omnibus holding this volume, rather than from anything said about this
	// book directly.
	Inherited bool `json:"inherited"`
}

// myBookBody matches the api's myBookBody. Every field is a pointer so an
// omitted one means "leave it alone": a tool that only sets a rating must not
// blank a review typed on another device.
type myBookBody struct {
	ReadStatus *string `json:"read_status,omitempty"`
	Rating     *int    `json:"rating,omitempty"`
	// ClearRating is separate because omitting a rating means "no change", so a
	// single nullable field cannot say both "leave it" and "remove it".
	ClearRating bool    `json:"clear_rating,omitempty"`
	IsFavorite  *bool   `json:"is_favorite,omitempty"`
	Review      *string `json:"review,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

// putMyBook writes the caller's state for a work and returns the stored row.
func putMyBook(ctx context.Context, client *api.Client, bookID string, body myBookBody) (readingState, error) {
	return api.Put[myBookBody, readingState](ctx, client, "/api/v1/books/"+bookID+"/me", body)
}

// ─── set_read_status ────────────────────────────────────────────────────────

type setReadStatusArgs struct {
	BookID string `json:"book_id" jsonschema:"uuid from search_books"`
	Status string `json:"status" jsonschema:"one of: unread, reading, read, did_not_finish"`
}

type setReadStatusResult struct {
	BookID     string `json:"book_id"`
	ReadStatus string `json:"read_status"`
	Message    string `json:"message"`
}

var validReadStatuses = map[string]bool{
	"unread":         true,
	"reading":        true,
	"read":           true,
	"did_not_finish": true,
}

// AddSetReadStatus wires the set_read_status tool.
func AddSetReadStatus(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_read_status",
		Description: "Update the caller's read status for a book. Status is one of unread/reading/read/did_not_finish. Reading state belongs to the work, so it applies however many copies or editions the caller owns. Leaves rating, notes, review and favourite untouched.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args setReadStatusArgs) (*mcp.CallToolResult, setReadStatusResult, error) {
		if args.BookID == "" || args.Status == "" {
			return nil, setReadStatusResult{}, fmt.Errorf("book_id and status are both required")
		}
		status := strings.ToLower(strings.TrimSpace(args.Status))
		if !validReadStatuses[status] {
			return nil, setReadStatusResult{}, fmt.Errorf("status must be one of unread, reading, read, did_not_finish (got %q)", args.Status)
		}

		updated, err := putMyBook(ctx, client, args.BookID, myBookBody{ReadStatus: &status})
		if err != nil {
			return nil, setReadStatusResult{}, fmt.Errorf("saving read status: %w", err)
		}
		return nil, setReadStatusResult{
			BookID:     updated.BookID,
			ReadStatus: updated.ReadStatus,
			Message:    fmt.Sprintf("Read status set to %q.", updated.ReadStatus),
		}, nil
	})
}

// ─── set_rating ─────────────────────────────────────────────────────────────

type setRatingArgs struct {
	BookID string `json:"book_id" jsonschema:"uuid from search_books"`
	Rating *int   `json:"rating" jsonschema:"1–10 (half-star UI: 2=1★, 4=2★, …, 10=5★); pass null to clear"`
}

type setRatingResult struct {
	BookID  string `json:"book_id"`
	Rating  *int   `json:"rating"`
	Message string `json:"message"`
}

// AddSetRating wires the set_rating tool. Rating is the 1–10 half-star integer
// the iOS and web UIs use; null clears the rating entirely.
func AddSetRating(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_rating",
		Description: "Set or clear the caller's rating for a book. Rating is an integer 1–10 matching the half-star UI (2=1★, 4=2★, 6=3★, 8=4★, 10=5★); pass null to clear. Leaves read status, notes, review and favourite untouched.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args setRatingArgs) (*mcp.CallToolResult, setRatingResult, error) {
		if args.BookID == "" {
			return nil, setRatingResult{}, fmt.Errorf("book_id is required")
		}
		if args.Rating != nil && (*args.Rating < 1 || *args.Rating > 10) {
			return nil, setRatingResult{}, fmt.Errorf("rating must be between 1 and 10 (got %d)", *args.Rating)
		}

		// The tool's contract is "null clears", so a nil rating has to become an
		// explicit clear rather than an omission, which the server reads as "no
		// change".
		body := myBookBody{Rating: args.Rating, ClearRating: args.Rating == nil}
		updated, err := putMyBook(ctx, client, args.BookID, body)
		if err != nil {
			return nil, setRatingResult{}, fmt.Errorf("saving rating: %w", err)
		}
		msg := "Rating cleared."
		if updated.Rating != nil {
			msg = fmt.Sprintf("Rating set to %d/10.", *updated.Rating)
		}
		return nil, setRatingResult{
			BookID:  updated.BookID,
			Rating:  updated.Rating,
			Message: msg,
		}, nil
	})
}

// ─── write_review ───────────────────────────────────────────────────────────

type writeReviewArgs struct {
	BookID     string  `json:"book_id" jsonschema:"uuid from search_books"`
	Notes      *string `json:"notes,omitempty" jsonschema:"private-to-you notes; pass empty string to clear, omit to leave unchanged"`
	Review     *string `json:"review,omitempty" jsonschema:"review visible to other library members; pass empty string to clear, omit to leave unchanged"`
	IsFavorite *bool   `json:"is_favorite,omitempty" jsonschema:"toggle favourite; omit to leave unchanged"`
}

type writeReviewResult struct {
	BookID     string `json:"book_id"`
	Notes      string `json:"notes"`
	Review     string `json:"review"`
	IsFavorite bool   `json:"is_favorite"`
	Message    string `json:"message"`
}

// AddWriteReview wires the write_review tool. Every field is optional: omitted
// fields stay as they were, explicit empty strings clear them. Notes are
// private to the user; review is visible to other library members.
func AddWriteReview(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "write_review",
		Description: "Write or update the caller's notes, review, or favourite flag for a book. Notes are private to the caller; review is visible to other members of any library the book is in. Any field omitted is preserved as-is; pass an empty string to clear notes or review. Leaves read status and rating untouched.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args writeReviewArgs) (*mcp.CallToolResult, writeReviewResult, error) {
		if args.BookID == "" {
			return nil, writeReviewResult{}, fmt.Errorf("book_id is required")
		}
		if args.Notes == nil && args.Review == nil && args.IsFavorite == nil {
			return nil, writeReviewResult{}, fmt.Errorf("at least one of notes, review, or is_favorite must be provided")
		}

		body := myBookBody{Notes: args.Notes, Review: args.Review, IsFavorite: args.IsFavorite}
		updated, err := putMyBook(ctx, client, args.BookID, body)
		if err != nil {
			return nil, writeReviewResult{}, fmt.Errorf("saving review: %w", err)
		}
		return nil, writeReviewResult{
			BookID:     updated.BookID,
			Notes:      updated.Notes,
			Review:     updated.Review,
			IsFavorite: updated.IsFavorite,
			Message:    "Saved.",
		}, nil
	})
}
