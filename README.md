# librarium-mcp

Model Context Protocol server for **[Librarium](https://librarium.press)** — a self-hosted, privacy-focused tracker for your physical book, manga, and comic collection. A self-hosted alternative to Libib and similar cloud catalog services.

Chat with your library from Claude Desktop, Cursor, Claude Code, or any MCP-aware client. Go · streamable HTTP. Talks to [`librarium-api`](https://github.com/fireball1725/librarium-api) like any other client.

> ⚠︎ **Early beta.** Things are changing fast, some edges are rough, and self-hosters should expect to read release notes before upgrading.

Part of the Librarium stack:

| Repo                                                                              | Role                                                                       |
| --------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| [`librarium`](https://github.com/fireball1725/librarium)                          | Marketing site at [librarium.press](https://librarium.press), planning docs |
| [`librarium-api`](https://github.com/fireball1725/librarium-api)                  | Backend · Go · Postgres · River jobs                                       |
| [`librarium-web`](https://github.com/fireball1725/librarium-web)                  | Web client · React · TypeScript · Tailwind · Vite                          |
| [`librarium-ios`](https://github.com/fireball1725/librarium-ios)                  | Native iOS client · SwiftUI · iOS 26+ (TestFlight)                         |
| [`librarium-mcp`](https://github.com/fireball1725/librarium-mcp) ← **you are here** | MCP server · Go · chat with your library from Claude / Cursor / etc.     |

## How it works

`librarium-mcp` is a standalone Go service that speaks MCP over streamable HTTP. It's just another Librarium API client, same as `librarium-web` and `librarium-ios` — no special backend access. MCP clients connect, authenticate with an inbound token, and call tools; each tool translates into an authenticated request against your Librarium API using the personal access token you mint from the web UI.

## Tools

Every tool inherits the permissions of the `lbrm_pat_*` token you configured, so narrowing the token's scope in the web UI narrows what the LLM can do here. Write tools are gated by the MCP client's per-call approval prompt on top of that.

### Read

| Tool | Args | Returns |
|---|---|---|
| `list_libraries` | — | Libraries the current user can access (id, name, description). |
| `search_books` | `query`, `library_id?`, `limit?` (default 25, max 100) | Compact book summaries. Scopes to one library or fans out across all. |
| `get_book` | `book_id` | Full book record: contributors, tags, genres, series, libraries, your read status. |
| `lookup_isbn` | `isbn` | Provider-merged catalog lookup (Google Books, Open Library, Hardcover, …). Same call the iOS scan flow uses. |
| `get_recent_suggestions` | `limit?` (default 5, max 25) | Recent AI suggestion runs with their books, reasoning, and per-suggestion status. |
| `list_loans` | `library_id`, `book_id?`, `include_returned?` | Active-only by default; opt-in to returned, narrow to one book. |

### Write

| Tool | Args | Returns |
|---|---|---|
| `add_book_by_isbn` | `isbn`, `library_id`, `media_type` (novel/manga/comic/…), `format` (paperback/hardcover/ebook/audiobook) | New `{book_id, edition_id}`. Triggers metadata + cover enrichment asynchronously. |
| `set_read_status` | `book_id`, `library_id`, `status` (unread/reading/read/did_not_finish), `edition_id?` | Updated interaction. |
| `set_rating` | `book_id`, `library_id`, `rating` (1–10 half-star integer, or null to clear), `edition_id?` | Updated interaction. |
| `write_review` | `book_id`, `library_id`, `notes?`, `review?`, `is_favorite?`, `edition_id?` | Updated interaction. Notes are private; review is visible to other library members. |
| `create_loan` | `library_id`, `book_id`, `loaned_to`, `loaned_at?`, `due_date?`, `notes?` | Records a book lent to someone. `loaned_at` defaults to today. |
| `mark_loan_returned` | `library_id`, `loan_id`, `returned_at?` | Marks an active loan returned. Preserves borrower / due date / notes / tags. |
| `delete_loan` | `library_id`, `loan_id` | Removes a loan record entirely. Prefer `mark_loan_returned` for normal returns. |

Write tools auto-resolve `edition_id` to the book's primary edition when it's omitted, and use a read-merge-write pattern against the api so a partial update doesn't clobber fields the caller didn't touch.

## Resources

Resources are read-only catalog views the LLM (or end-user via `/resource` UI) can pull on demand without consuming a tool-call slot. Useful for "show me what's in my library" prompts where a tool would be wasteful. Same outbound auth as tools — every resource respects the configured `lbrm_pat_*` token's scope.

| URI | Returns |
|---|---|
| `librarium://libraries` | Every library the current user can access. |
| `librarium://library/{id}` | Single library metadata (no book list). |
| `librarium://library/{id}/books` | First page of books in a library. Use `search_books` for filtering. |
| `librarium://library/{id}/series` | Every series tracked in a library. |
| `librarium://library/{id}/loans` | Every active loan in a library. |
| `librarium://library/{lib}/series/{sid}` | Single series detail with status, total volumes, and arcs. |
| `librarium://book/{id}` | Single book detail (library-agnostic). |
| `librarium://book/{id}/loans` | Every loan ever recorded for a book (active + returned), across every library the user can see. |
| `librarium://suggestions/recent` | Most recent AI suggestions with lifecycle status. |
| `librarium://stats` | Aggregate counts across every library the current user can see. |

All resources return `application/json`. Templated URIs use RFC 6570 `{name}` placeholders.

## Quick start

1. In your Librarium web UI, open **Profile → API tokens** and create a new token. Copy the raw value.
2. Drop an `mcp` service into your existing `docker-compose.yml`:

   ```yaml
   services:
     mcp:
       image: ghcr.io/fireball1725/librarium-mcp:latest
       restart: unless-stopped
       ports:
         - "8090:8090"
       environment:
         LIBRARIUM_API_URL: https://librarium.example.com
         LIBRARIUM_ACCESS_TOKEN: lbrm_pat_...   # from step 1
       volumes:
         - ./mcp-data:/data
   ```

3. Start it: `docker compose up -d mcp`. On first boot the server generates an inbound MCP token, prints it in the log, and persists it to `/data/mcp-token`. Copy that value.
4. Add the server to your MCP client (e.g. Claude Desktop's `claude_desktop_config.json`):

   ```json
   {
     "mcpServers": {
       "librarium": {
         "url": "http://localhost:8090/mcp",
         "headers": {
           "Authorization": "Bearer lbrm_mcp_..."
         }
       }
     }
   }
   ```

## Configuration

| Env var | Required | Default | Notes |
|---|---|---|---|
| `LIBRARIUM_API_URL` | yes | — | Base URL of your Librarium API (no trailing slash). |
| `LIBRARIUM_ACCESS_TOKEN` | yes | — | `lbrm_pat_*` token minted in the web UI. |
| `LIBRARIUM_MCP_LISTEN` | no | `:8090` | Bind address for the MCP server. |
| `LIBRARIUM_MCP_TOKEN` | no | auto | Inbound bearer token. If unset, checks `$LIBRARIUM_MCP_DATA_DIR/mcp-token`; if still missing, generates one and persists it. Env var wins over the file. |
| `LIBRARIUM_MCP_DATA_DIR` | no | `/data` | Where the persisted token lives. |

## Development

```sh
go mod tidy
LIBRARIUM_API_URL=http://localhost:8080 \
LIBRARIUM_ACCESS_TOKEN=lbrm_pat_... \
LIBRARIUM_MCP_DATA_DIR=./data \
  go run ./cmd/mcp
```

Health check: `curl -H "Authorization: Bearer <mcp-token>" http://localhost:8090/health`.

## License

AGPL-3.0-only, matching the rest of the Librarium stack.
