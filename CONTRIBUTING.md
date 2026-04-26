# Contributing to librarium-mcp

Thanks for your interest in contributing. This document covers how to submit changes, what's expected in a PR, and the legal terms your contribution is made under.

## Before you start

- Check the open issues — your change may already be in progress or planned differently.
- For anything non-trivial, open an issue to discuss before writing code.
- The MCP server is a thin shim over the [librarium-api](https://github.com/fireball1725/librarium-api). Tools should map cleanly to API endpoints — don't reimplement business logic here.
- Self-hosted is the primary deployment target.

## Development setup

The everyday dev stack lives in the umbrella [`librarium`](https://github.com/fireball1725/librarium) workspace under `local/docker-compose.yml` (api + web + db + mcp). After any code change to `mcp/`, rebuild and restart:

```bash
cd local && docker compose up -d --build mcp
```

You'll need a Librarium personal access token in your env (mint one in the web UI under Profile → API tokens):

```bash
export LIBRARIUM_ACCESS_TOKEN=lbrm_pat_...
```

For running Go tooling directly (`go test`, `go vet`), install Go 1.25.

## Making changes

- Keep changes focused. One PR = one feature/fix.
- Add or update tests for any behaviour change.
- Run `go vet ./...` and `go test ./...` before submitting.
- New tools should follow the existing naming pattern (`<verb>_<noun>`, e.g. `add_book_by_isbn`, `set_rating`) and have clear `description` strings — those become the model-facing docs.

## Commit messages

Short, imperative, reference the scope: `feat(tools): add list_libraries`, `fix(http): retry transient 502s once`, `chore(ci): pin go-version`.

## Pull requests

- Rebase on `main` before opening the PR.
- Title carries the weight — it feeds auto-generated release notes.
- Body: 1–2 terse bullets explaining the why. No Summary/Test plan headers.
- CI must pass before review.
- Don't hand-edit a `CHANGELOG.md` — release notes are auto-generated from PR titles.

## License

The project is licensed under the **GNU Affero General Public License v3.0 only** ([LICENSE](./LICENSE)). Contributions are accepted under the same license — nothing is assigned to the maintainer.

## Sign your commits (DCO)

Every commit in a pull request must carry a `Signed-off-by:` trailer certifying the [Developer Certificate of Origin 1.1](./DCO). Pass `-s` to `git commit`:

```bash
git commit -s -m "feat(tools): add list_libraries"
```

If you forget on one commit: `git commit --amend -s --no-edit`. For several: `git rebase --signoff main`.

The [DCO GitHub App](https://github.com/apps/dco) runs on every PR and blocks the merge if any commit is missing a sign-off.

## Code of conduct

Be decent. Assume good faith. Technical disagreements are fine; personal attacks aren't.
