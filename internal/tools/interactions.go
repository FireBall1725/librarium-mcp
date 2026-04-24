// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The three write tools in this file (set_read_status, set_rating,
// write_review) all operate on the api's my-interaction endpoint. That
// endpoint is a full upsert: a PUT with a partial body wipes the fields
// the body omits. So every tool here does GET → merge caller-supplied
// fields with the existing row → PUT, which gives the LLM partial-update
// semantics without surprises.
//
// edition_id is optional on every tool. When the caller omits it we pull
// the editions list for the book and fall back to the primary edition
// (or the first one if none is marked primary). That mirrors the iOS
// client's default "operate on the primary edition" behaviour and keeps
// tool calls short in the common case of "I only own one edition".

// ─── Shared shapes ──────────────────────────────────────────────────────────

// apiEditionRow is the edition listing row. We only care about id +
// is_primary for edition resolution; the rest is ignored.
type apiEditionRow struct {
	ID        string `json:"id"`
	IsPrimary bool   `json:"is_primary"`
}

// apiInteraction mirrors the interactionBody map in api/handlers/editions.go.
// Fields we don't touch (user_id, created_at) are omitted so we never accidentally
// mangle them on upsert.
type apiInteraction struct {
	ID            string `json:"id,omitempty"`
	BookEditionID string `json:"book_edition_id,omitempty"`
	ReadStatus    string `json:"read_status,omitempty"`
	Rating        *int   `json:"rating"`
	Notes         string `json:"notes"`
	Review        string `json:"review"`
	DateStarted   string `json:"date_started,omitempty"`
	DateFinished  string `json:"date_finished,omitempty"`
	IsFavorite    bool   `json:"is_favorite"`
	RereadCount   int    `json:"reread_count,omitempty"`
}

// interactionPutBody matches the api's decodeInteractionRequest shape.
// Matching this exactly (vs sending the full apiInteraction) avoids the
// api rejecting extra fields or reinterpreting read-only ones like id.
type interactionPutBody struct {
	ReadStatus   string `json:"read_status"`
	Rating       *int   `json:"rating"`
	Notes        string `json:"notes"`
	Review       string `json:"review"`
	DateStarted  string `json:"date_started,omitempty"`
	DateFinished string `json:"date_finished,omitempty"`
	IsFavorite   bool   `json:"is_favorite"`
}

// resolveEditionID returns the edition_id to operate on. If the caller
// supplied one we trust it; otherwise we pick the primary edition, or
// fall back to the first one if the book has no primary marker set.
func resolveEditionID(ctx context.Context, client *api.Client, libraryID, bookID, supplied string) (string, error) {
	if supplied != "" {
		return supplied, nil
	}
	path := fmt.Sprintf("/api/v1/libraries/%s/books/%s/editions", libraryID, bookID)
	editions, err := api.Get[[]apiEditionRow](ctx, client, path)
	if err != nil {
		return "", fmt.Errorf("listing editions: %w", err)
	}
	if len(editions) == 0 {
		return "", errors.New("book has no editions; cannot set interaction")
	}
	for _, e := range editions {
		if e.IsPrimary {
			return e.ID, nil
		}
	}
	return editions[0].ID, nil
}

// loadInteraction GETs the current interaction. The api returns a null
// body when the user has no interaction row yet, which decodes into a
// zero-value apiInteraction — that's the starting point for first writes.
func loadInteraction(ctx context.Context, client *api.Client, libraryID, bookID, editionID string) (apiInteraction, error) {
	path := fmt.Sprintf("/api/v1/libraries/%s/books/%s/editions/%s/my-interaction",
		libraryID, bookID, editionID)
	cur, err := api.Get[apiInteraction](ctx, client, path)
	if err != nil {
		var apiErr *api.Error
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return apiInteraction{}, nil
		}
		return apiInteraction{}, err
	}
	return cur, nil
}

// putInteraction writes the merged body back. Returns the refreshed row
// so each tool can surface its post-write state to the LLM.
func putInteraction(ctx context.Context, client *api.Client, libraryID, bookID, editionID string, body interactionPutBody) (apiInteraction, error) {
	path := fmt.Sprintf("/api/v1/libraries/%s/books/%s/editions/%s/my-interaction",
		libraryID, bookID, editionID)
	return api.Put[interactionPutBody, apiInteraction](ctx, client, path, body)
}

// mergeBase turns the current api row into a fresh interactionPutBody,
// giving each setter a "carry existing fields forward" starting point
// so we don't clobber values the caller didn't mention.
func mergeBase(cur apiInteraction) interactionPutBody {
	status := cur.ReadStatus
	if status == "" {
		status = "unread" // matches the api's own default
	}
	return interactionPutBody{
		ReadStatus:   status,
		Rating:       cur.Rating,
		Notes:        cur.Notes,
		Review:       cur.Review,
		DateStarted:  cur.DateStarted,
		DateFinished: cur.DateFinished,
		IsFavorite:   cur.IsFavorite,
	}
}

// ─── set_read_status ────────────────────────────────────────────────────────

type setReadStatusArgs struct {
	BookID    string `json:"book_id" jsonschema:"uuid from search_books"`
	LibraryID string `json:"library_id" jsonschema:"uuid of the library containing the book"`
	Status    string `json:"status" jsonschema:"one of: unread, reading, read, did_not_finish"`
	EditionID string `json:"edition_id,omitempty" jsonschema:"optional; defaults to the book's primary edition"`
}

type setReadStatusResult struct {
	EditionID  string `json:"edition_id"`
	ReadStatus string `json:"read_status"`
	Message    string `json:"message"`
}

var validReadStatuses = map[string]bool{
	"unread":         true,
	"reading":        true,
	"read":           true,
	"did_not_finish": true,
}

// AddSetReadStatus wires the set_read_status tool. Picks the primary
// edition when the caller doesn't specify one.
func AddSetReadStatus(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_read_status",
		Description: "Update the caller's read status for a book in one of their libraries. Status is one of unread/reading/read/did_not_finish. If edition_id is omitted the primary edition is used. Preserves other interaction fields (rating, notes, review, favourite).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args setReadStatusArgs) (*mcp.CallToolResult, setReadStatusResult, error) {
		if args.BookID == "" || args.LibraryID == "" || args.Status == "" {
			return nil, setReadStatusResult{}, fmt.Errorf("book_id, library_id, and status are all required")
		}
		status := strings.ToLower(strings.TrimSpace(args.Status))
		if !validReadStatuses[status] {
			return nil, setReadStatusResult{}, fmt.Errorf("status must be one of unread, reading, read, did_not_finish (got %q)", args.Status)
		}

		editionID, err := resolveEditionID(ctx, client, args.LibraryID, args.BookID, args.EditionID)
		if err != nil {
			return nil, setReadStatusResult{}, err
		}
		cur, err := loadInteraction(ctx, client, args.LibraryID, args.BookID, editionID)
		if err != nil {
			return nil, setReadStatusResult{}, fmt.Errorf("loading current interaction: %w", err)
		}
		body := mergeBase(cur)
		body.ReadStatus = status

		updated, err := putInteraction(ctx, client, args.LibraryID, args.BookID, editionID, body)
		if err != nil {
			return nil, setReadStatusResult{}, fmt.Errorf("saving interaction: %w", err)
		}
		return nil, setReadStatusResult{
			EditionID:  editionID,
			ReadStatus: updated.ReadStatus,
			Message:    fmt.Sprintf("Read status set to %q.", updated.ReadStatus),
		}, nil
	})
}

// ─── set_rating ─────────────────────────────────────────────────────────────

type setRatingArgs struct {
	BookID    string `json:"book_id" jsonschema:"uuid from search_books"`
	LibraryID string `json:"library_id" jsonschema:"uuid of the library containing the book"`
	Rating    *int   `json:"rating" jsonschema:"1–10 (half-star UI: 2=1★, 4=2★, …, 10=5★); pass null to clear"`
	EditionID string `json:"edition_id,omitempty" jsonschema:"optional; defaults to the book's primary edition"`
}

type setRatingResult struct {
	EditionID string `json:"edition_id"`
	Rating    *int   `json:"rating"`
	Message   string `json:"message"`
}

// AddSetRating wires the set_rating tool. Rating is the 1–10 half-star
// integer the iOS and web UIs use; null clears the rating entirely.
func AddSetRating(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "set_rating",
		Description: "Set or clear the caller's rating for a book. Rating is an integer 1–10 matching the half-star UI (2=1★, 4=2★, 6=3★, 8=4★, 10=5★); pass null to clear. Preserves read_status, notes, review, favourite.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args setRatingArgs) (*mcp.CallToolResult, setRatingResult, error) {
		if args.BookID == "" || args.LibraryID == "" {
			return nil, setRatingResult{}, fmt.Errorf("book_id and library_id are required")
		}
		if args.Rating != nil {
			if *args.Rating < 1 || *args.Rating > 10 {
				return nil, setRatingResult{}, fmt.Errorf("rating must be between 1 and 10 (got %d)", *args.Rating)
			}
		}

		editionID, err := resolveEditionID(ctx, client, args.LibraryID, args.BookID, args.EditionID)
		if err != nil {
			return nil, setRatingResult{}, err
		}
		cur, err := loadInteraction(ctx, client, args.LibraryID, args.BookID, editionID)
		if err != nil {
			return nil, setRatingResult{}, fmt.Errorf("loading current interaction: %w", err)
		}
		body := mergeBase(cur)
		body.Rating = args.Rating

		updated, err := putInteraction(ctx, client, args.LibraryID, args.BookID, editionID, body)
		if err != nil {
			return nil, setRatingResult{}, fmt.Errorf("saving interaction: %w", err)
		}
		msg := "Rating cleared."
		if updated.Rating != nil {
			msg = fmt.Sprintf("Rating set to %d/10.", *updated.Rating)
		}
		return nil, setRatingResult{
			EditionID: editionID,
			Rating:    updated.Rating,
			Message:   msg,
		}, nil
	})
}

// ─── write_review ───────────────────────────────────────────────────────────

type writeReviewArgs struct {
	BookID     string  `json:"book_id" jsonschema:"uuid from search_books"`
	LibraryID  string  `json:"library_id" jsonschema:"uuid of the library containing the book"`
	Notes      *string `json:"notes,omitempty" jsonschema:"private-to-you notes; pass empty string to clear, omit to leave unchanged"`
	Review     *string `json:"review,omitempty" jsonschema:"review visible to other library members; pass empty string to clear, omit to leave unchanged"`
	IsFavorite *bool   `json:"is_favorite,omitempty" jsonschema:"toggle favourite; omit to leave unchanged"`
	EditionID  string  `json:"edition_id,omitempty" jsonschema:"optional; defaults to the book's primary edition"`
}

type writeReviewResult struct {
	EditionID  string `json:"edition_id"`
	Notes      string `json:"notes"`
	Review     string `json:"review"`
	IsFavorite bool   `json:"is_favorite"`
	Message    string `json:"message"`
}

// AddWriteReview wires the write_review tool. Every field is optional —
// omitted fields stay as they were, explicit empty strings clear them.
// Notes are private to the user; review is visible to other library
// members (see plans/librarium-mcp.md for the cross-user-review gap).
func AddWriteReview(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "write_review",
		Description: "Write or update the caller's notes, review, or favourite flag for a book. Notes are private to the caller; review is visible to other members of the library. Any field omitted is preserved as-is; pass an empty string to clear notes or review. Preserves read_status and rating.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args writeReviewArgs) (*mcp.CallToolResult, writeReviewResult, error) {
		if args.BookID == "" || args.LibraryID == "" {
			return nil, writeReviewResult{}, fmt.Errorf("book_id and library_id are required")
		}
		if args.Notes == nil && args.Review == nil && args.IsFavorite == nil {
			return nil, writeReviewResult{}, fmt.Errorf("at least one of notes, review, or is_favorite must be provided")
		}

		editionID, err := resolveEditionID(ctx, client, args.LibraryID, args.BookID, args.EditionID)
		if err != nil {
			return nil, writeReviewResult{}, err
		}
		cur, err := loadInteraction(ctx, client, args.LibraryID, args.BookID, editionID)
		if err != nil {
			return nil, writeReviewResult{}, fmt.Errorf("loading current interaction: %w", err)
		}
		body := mergeBase(cur)
		if args.Notes != nil {
			body.Notes = *args.Notes
		}
		if args.Review != nil {
			body.Review = *args.Review
		}
		if args.IsFavorite != nil {
			body.IsFavorite = *args.IsFavorite
		}

		updated, err := putInteraction(ctx, client, args.LibraryID, args.BookID, editionID, body)
		if err != nil {
			return nil, writeReviewResult{}, fmt.Errorf("saving interaction: %w", err)
		}
		return nil, writeReviewResult{
			EditionID:  editionID,
			Notes:      updated.Notes,
			Review:     updated.Review,
			IsFavorite: updated.IsFavorite,
			Message:    "Saved.",
		}, nil
	})
}
