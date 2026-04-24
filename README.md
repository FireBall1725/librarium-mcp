# librarium-mcp

Model Context Protocol server for [Librarium](https://librarium.press). Chat with your self-hosted library from Claude Desktop, Cursor, Claude Code, or any MCP-aware client.

> ⚠︎ **Early beta.** v1 ships the tool catalogue below; MCP resources are deferred to v1.1.

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

### Write

| Tool | Args | Returns |
|---|---|---|
| `add_book_by_isbn` | `isbn`, `library_id`, `media_type` (novel/manga/comic/…), `format` (paperback/hardcover/ebook/audiobook) | New `{book_id, edition_id}`. Triggers metadata + cover enrichment asynchronously. |
| `set_read_status` | `book_id`, `library_id`, `status` (unread/reading/read/did_not_finish), `edition_id?` | Updated interaction. |
| `set_rating` | `book_id`, `library_id`, `rating` (1–10 half-star integer, or null to clear), `edition_id?` | Updated interaction. |
| `write_review` | `book_id`, `library_id`, `notes?`, `review?`, `is_favorite?`, `edition_id?` | Updated interaction. Notes are private; review is visible to other library members. |

Write tools auto-resolve `edition_id` to the book's primary edition when it's omitted, and use a read-merge-write pattern against the api so a partial update doesn't clobber fields the caller didn't touch.

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
