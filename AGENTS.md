# AGENTS.md

Guidance for AI coding agents working in `librarium-mcp`. Humans should read
[CONTRIBUTING.md](./CONTRIBUTING.md) first; this file assumes you have and does
not repeat it.

## What this repo is

An MCP server that puts a Librarium library in front of an AI assistant, so
Claude Desktop, Cursor, or Claude Code can search a collection, add a book by
ISBN, track loans, and set read status without anyone writing API client code.

Librarium is a self-hosted, privacy-focused tracker for physical book, manga,
and comic collections, built across five repos that ship independently:

| Repo | Role |
| --- | --- |
| [`librarium`](https://github.com/FireBall1725/librarium) | Marketing site at librarium.press |
| [`librarium-api`](https://github.com/FireBall1725/librarium-api) | Go backend, the contract this server calls |
| [`librarium-web`](https://github.com/FireBall1725/librarium-web) | React client |
| [`librarium-ios`](https://github.com/FireBall1725/librarium-ios) | SwiftUI client |
| **`librarium-mcp`** | **This repo. Go, MCP over streamable HTTP** |

This repo versions on its own and is not pinned to an api release.

## Product rules that shape design decisions

- **This is a client, not a second backend.** Every tool is a thin wrapper over
  a `librarium-api` endpoint. If a tool needs data the API does not expose, the
  fix is an API endpoint, not a query or a join here.
- **Self-hosted is canon.** The server talks to whatever instance the operator
  points it at. No hosted service, no phoning home.
- **A personal access token is the whole auth model.** The operator mints a PAT
  in the Librarium web UI and passes it as `LIBRARIUM_ACCESS_TOKEN`. Never log
  it, never echo it in an error, never write it to disk.
- **Write tools change a real person's library.** Reads can be liberal; writes
  need clear tool descriptions so the model knows what it is about to do.

## Stack

Go 1.25 and the official
[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk),
served over streamable HTTP with `mcp.NewStreamableHTTPHandler`. No other
runtime dependencies.

## Layout

```
cmd/main.go              config, HTTP handler, server startup
internal/
  api/                   HTTP client for librarium-api. One method per endpoint
  tools/                 one file per tool group:
                           books.go        search, get, list libraries
                           isbn.go         ISBN lookup
                           add.go          add a book by ISBN
                           interactions.go read status, rating, review
                           loans.go        create, list, return, delete
                           suggestions.go  recent AI suggestions
                           tools.go        RegisterAll, the single wiring point
  resources/             MCP resources
  config/                environment parsing
  version/               injected at release, 0.0.0-dev otherwise
```

Adding a tool means a new `Add*` function in the right group file plus one line
in `RegisterAll`. Keep that pattern; it is the reason `main.go` has stayed
small.

## Build and test

The server needs a running Librarium instance and a PAT:

```bash
export LIBRARIUM_API_URL=http://localhost:8080
export LIBRARIUM_ACCESS_TOKEN=lbrm_pat_...
go run ./cmd
```

The umbrella [`librarium`](https://github.com/FireBall1725/librarium) workspace
runs this alongside the api and db in `local/docker-compose.yml`.

Before opening a PR, run what CI runs:

```bash
go build ./...
go vet ./...
gofmt -l .              # must print nothing
go test -race -count=1 ./...
golangci-lint run
```

CI also runs `editorconfig-checker` and a Docker build. The jobs live in
[FireBall1725/workflows](https://github.com/FireBall1725/workflows), shared with
the other Go repo here, so change them there rather than in this repo's
`ci.yml`.

## Things that will bite you

- **Tool descriptions are the interface.** The model picks a tool from its
  description and parameter docs alone. A vague description is a bug, and it
  shows up as the assistant calling the wrong tool rather than as a test
  failure.
- **Never hand-edit the version.** `internal/version` reports `0.0.0-dev` and
  the release workflow injects the real `YY.M.R` via ldflags.
- **Never edit `CHANGELOG.md`.** Release notes are generated from PR titles.
- **Errors reach a language model, not a log file.** An error string like
  `401 Unauthorized` tells the assistant nothing useful. Say what failed and
  what the operator should check.
- **Do not widen a tool's blast radius quietly.** A tool that deletes or
  overwrites needs that stated in its description.

## Conventions

- Every file starts with the SPDX header and copyright line already used
  throughout the repo. Copy the form from a neighbouring file.
- Comments explain why. The existing files carry rationale above non-obvious
  decisions; match that density.
- Errors wrap with context: `fmt.Errorf("looking up ISBN: %w", err)`.
- Log with `slog`, structured key/value pairs, and never include the token.
- Commit messages are short and imperative with a scope:
  `feat(loans): add mark-returned tool`.
- Every commit needs a DCO sign-off (`git commit -s`).
