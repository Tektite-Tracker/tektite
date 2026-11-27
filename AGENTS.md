# AGENTS.md

Guide for anyone, human or agent, changing this repo. See README.md for what tektite is.

## Layout

- `parse/`: public package. Pure functions from bytes to nodes, events and diagnostics. No I/O.
- `internal/node`, `internal/event`, `internal/store`, `internal/check`: node model, event folding, on-disk store, policy checks. I/O belongs here, never in `parse`.
- `cmd/tkt`: the CLI entry point.
- `spec/`: Gherkin feature files, the source of truth for behaviour.
- `testdata/`: golden fixtures.
- `tools/commentlint`: the golangci-lint plugin that enforces the comment policy.

## Commands

- `make lint`: builds the custom golangci-lint binary and runs it.
- `make test`: `go test ./...`.
- `make build`: builds `bin/tkt`.
- `scripts/spec-coverage.sh`: lists spec scenarios that no test names.

Run lint, test and build before pushing.

## Tests

Go tests plus golden files under `testdata/`. Every test names the spec scenario it covers by using the scenario title as the `t.Run` name, which is what the coverage script greps for. Add or change the scenario in `spec/` first when behaviour changes.

## Comments

Only three kinds are allowed: docblocks on declarations, tool directives, and comments starting with `why:` that explain a non-obvious decision. Everything else is rejected by commentlint.

## Commits and PRs

- One branch and PR per piece of work, branched from an up-to-date `main`, named `{type}/{short_desc}` in lowercase, e.g. `feat/node-event-fold`.
- Commits are a single Conventional Commit line (`feat:`, `fix:`, `chore:`, `docs:`), no body, no attribution trailers.
- PR descriptions are a one-line tl;dr and one or two short paragraphs, no attribution.
- Wait for CI to pass. Do not merge your own PR.
