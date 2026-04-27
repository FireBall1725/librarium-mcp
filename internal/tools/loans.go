// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 FireBall1725 (Adaléa)

package tools

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/fireball1725/librarium-mcp/internal/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ─── Shared shapes ──────────────────────────────────────────────────────────

// apiLoan is the loan body returned by the api list / patch endpoints.
type apiLoan struct {
	ID         string  `json:"id"`
	LibraryID  string  `json:"library_id"`
	BookID     string  `json:"book_id"`
	BookTitle  string  `json:"book_title"`
	LoanedTo   string  `json:"loaned_to"`
	LoanedAt   string  `json:"loaned_at"`
	DueDate    *string `json:"due_date"`
	ReturnedAt *string `json:"returned_at"`
	Notes      string  `json:"notes"`
}

// LoanSummary is the trimmed shape we hand back to the LLM. Drops fields
// the model rarely needs (created_at/updated_at) and renames the timestamps
// to make their semantics obvious.
type LoanSummary struct {
	ID         string  `json:"id"`
	BookID     string  `json:"book_id"`
	BookTitle  string  `json:"book_title"`
	LoanedTo   string  `json:"loaned_to"`
	LoanedAt   string  `json:"loaned_at"`
	DueDate    *string `json:"due_date,omitempty"`
	ReturnedAt *string `json:"returned_at,omitempty"`
	Notes      string  `json:"notes,omitempty"`
	IsActive   bool    `json:"is_active"`
}

func projectLoan(l apiLoan) LoanSummary {
	return LoanSummary{
		ID:         l.ID,
		BookID:     l.BookID,
		BookTitle:  l.BookTitle,
		LoanedTo:   l.LoanedTo,
		LoanedAt:   l.LoanedAt,
		DueDate:    l.DueDate,
		ReturnedAt: l.ReturnedAt,
		Notes:      l.Notes,
		IsActive:   l.ReturnedAt == nil,
	}
}

// loanPatchBody mirrors the api's UpdateLoan body. Every PATCH is a full
// rewrite, so callers must read the loan first and merge the field they
// want to change — see findLoanByID + markReturned below.
type loanPatchBody struct {
	LoanedTo   string  `json:"loaned_to"`
	DueDate    *string `json:"due_date"`
	ReturnedAt *string `json:"returned_at"`
	Notes      string  `json:"notes"`
}

// findLoanByID hits the list endpoint and picks out the matching row.
// Used by mark_loan_returned to load the current state for the merge.
// `include_returned=true` so already-returned loans are still findable.
func findLoanByID(ctx context.Context, client *api.Client, libraryID, loanID string) (*apiLoan, error) {
	path := fmt.Sprintf("/api/v1/libraries/%s/loans?include_returned=true", libraryID)
	loans, err := api.Get[[]apiLoan](ctx, client, path)
	if err != nil {
		return nil, fmt.Errorf("listing loans: %w", err)
	}
	for i := range loans {
		if loans[i].ID == loanID {
			return &loans[i], nil
		}
	}
	return nil, fmt.Errorf("loan %s not found in library %s", loanID, libraryID)
}

// ─── list_loans ─────────────────────────────────────────────────────────────

type listLoansArgs struct {
	LibraryID       string `json:"library_id" jsonschema:"uuid of the library to list loans from"`
	BookID          string `json:"book_id,omitempty" jsonschema:"optional; filter to loans of a single book"`
	IncludeReturned bool   `json:"include_returned,omitempty" jsonschema:"include loans that have been returned (default false)"`
}

type listLoansResult struct {
	Loans []LoanSummary `json:"loans"`
}

// AddListLoans wires the list_loans tool. Default is active-only loans for
// a library; opt in to returned via include_returned, narrow to one book
// via book_id.
func AddListLoans(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_loans",
		Description: "List loans for one library. Default is active-only (currently lent out). Pass include_returned=true to include returned loans, and book_id to narrow to a single book's loan history.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listLoansArgs) (*mcp.CallToolResult, listLoansResult, error) {
		if args.LibraryID == "" {
			return nil, listLoansResult{}, errors.New("library_id is required")
		}
		q := url.Values{}
		if args.IncludeReturned {
			q.Set("include_returned", "true")
		}
		if args.BookID != "" {
			q.Set("book_id", args.BookID)
		}
		path := fmt.Sprintf("/api/v1/libraries/%s/loans", args.LibraryID)
		if enc := q.Encode(); enc != "" {
			path += "?" + enc
		}
		loans, err := api.Get[[]apiLoan](ctx, client, path)
		if err != nil {
			return nil, listLoansResult{}, err
		}
		out := make([]LoanSummary, len(loans))
		for i, l := range loans {
			out[i] = projectLoan(l)
		}
		return nil, listLoansResult{Loans: out}, nil
	})
}

// ─── create_loan ────────────────────────────────────────────────────────────

type createLoanArgs struct {
	LibraryID string `json:"library_id" jsonschema:"uuid of the library that owns the book"`
	BookID    string `json:"book_id" jsonschema:"uuid of the book being lent"`
	LoanedTo  string `json:"loaned_to" jsonschema:"name or contact for the borrower"`
	LoanedAt  string `json:"loaned_at,omitempty" jsonschema:"YYYY-MM-DD; defaults to today"`
	DueDate   string `json:"due_date,omitempty" jsonschema:"YYYY-MM-DD; optional"`
	Notes     string `json:"notes,omitempty" jsonschema:"optional"`
}

type createLoanResult struct {
	Loan    LoanSummary `json:"loan"`
	Message string      `json:"message"`
}

type loanCreateBody struct {
	BookID   string  `json:"book_id"`
	LoanedTo string  `json:"loaned_to"`
	LoanedAt string  `json:"loaned_at"`
	DueDate  *string `json:"due_date,omitempty"`
	Notes    string  `json:"notes,omitempty"`
}

// AddCreateLoan wires the create_loan tool. Records that a book has been
// lent to someone. loaned_at defaults to today when omitted.
func AddCreateLoan(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_loan",
		Description: "Record a new loan: a book lent from one of the user's libraries to someone. Returns the new loan id. loaned_at defaults to today; due_date and notes are optional.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args createLoanArgs) (*mcp.CallToolResult, createLoanResult, error) {
		if args.LibraryID == "" || args.BookID == "" || strings.TrimSpace(args.LoanedTo) == "" {
			return nil, createLoanResult{}, errors.New("library_id, book_id, and loaned_to are all required")
		}
		body := loanCreateBody{
			BookID:   args.BookID,
			LoanedTo: args.LoanedTo,
			LoanedAt: args.LoanedAt,
			Notes:    args.Notes,
		}
		if body.LoanedAt == "" {
			body.LoanedAt = time.Now().Format("2006-01-02")
		}
		if args.DueDate != "" {
			d := args.DueDate
			body.DueDate = &d
		}
		path := fmt.Sprintf("/api/v1/libraries/%s/loans", args.LibraryID)
		created, err := api.Post[loanCreateBody, apiLoan](ctx, client, path, body)
		if err != nil {
			return nil, createLoanResult{}, err
		}
		return nil, createLoanResult{
			Loan:    projectLoan(created),
			Message: fmt.Sprintf("Loan recorded: %q lent to %s.", created.BookTitle, created.LoanedTo),
		}, nil
	})
}

// ─── mark_loan_returned ─────────────────────────────────────────────────────

type markLoanReturnedArgs struct {
	LibraryID  string `json:"library_id" jsonschema:"uuid of the library the loan belongs to"`
	LoanID     string `json:"loan_id" jsonschema:"uuid of the loan to mark returned"`
	ReturnedAt string `json:"returned_at,omitempty" jsonschema:"YYYY-MM-DD; defaults to today"`
}

type markLoanReturnedResult struct {
	Loan    LoanSummary `json:"loan"`
	Message string      `json:"message"`
}

// AddMarkLoanReturned wires the mark_loan_returned tool. Reads the
// current loan first (the api PATCH is a full rewrite, so we have to
// preserve loaned_to / due_date / notes / tags) and sets returned_at.
func AddMarkLoanReturned(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "mark_loan_returned",
		Description: "Mark an active loan as returned. returned_at defaults to today. Other loan fields (borrower, due date, notes, tags) are preserved.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args markLoanReturnedArgs) (*mcp.CallToolResult, markLoanReturnedResult, error) {
		if args.LibraryID == "" || args.LoanID == "" {
			return nil, markLoanReturnedResult{}, errors.New("library_id and loan_id are required")
		}
		cur, err := findLoanByID(ctx, client, args.LibraryID, args.LoanID)
		if err != nil {
			return nil, markLoanReturnedResult{}, err
		}
		returnedAt := args.ReturnedAt
		if returnedAt == "" {
			returnedAt = time.Now().Format("2006-01-02")
		}
		body := loanPatchBody{
			LoanedTo:   cur.LoanedTo,
			DueDate:    cur.DueDate,
			ReturnedAt: &returnedAt,
			Notes:      cur.Notes,
		}
		path := fmt.Sprintf("/api/v1/libraries/%s/loans/%s", args.LibraryID, args.LoanID)
		updated, err := api.Patch[loanPatchBody, apiLoan](ctx, client, path, body)
		if err != nil {
			return nil, markLoanReturnedResult{}, err
		}
		return nil, markLoanReturnedResult{
			Loan:    projectLoan(updated),
			Message: fmt.Sprintf("Marked %q returned on %s.", updated.BookTitle, returnedAt),
		}, nil
	})
}

// ─── delete_loan ────────────────────────────────────────────────────────────

type deleteLoanArgs struct {
	LibraryID string `json:"library_id" jsonschema:"uuid of the library the loan belongs to"`
	LoanID    string `json:"loan_id" jsonschema:"uuid of the loan to delete"`
}

type deleteLoanResult struct {
	Message string `json:"message"`
}

// AddDeleteLoan wires the delete_loan tool. Use mark_loan_returned to
// record a clean return; delete_loan is for removing erroneous entries.
func AddDeleteLoan(srv *mcp.Server, client *api.Client) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "delete_loan",
		Description: "Delete a loan record entirely. Prefer mark_loan_returned for normal returns; this is for removing entries that were created in error.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args deleteLoanArgs) (*mcp.CallToolResult, deleteLoanResult, error) {
		if args.LibraryID == "" || args.LoanID == "" {
			return nil, deleteLoanResult{}, errors.New("library_id and loan_id are required")
		}
		path := fmt.Sprintf("/api/v1/libraries/%s/loans/%s", args.LibraryID, args.LoanID)
		if err := api.Delete(ctx, client, path); err != nil {
			return nil, deleteLoanResult{}, err
		}
		return nil, deleteLoanResult{
			Message: fmt.Sprintf("Loan %s deleted.", args.LoanID),
		}, nil
	})
}
