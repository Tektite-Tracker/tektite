---
name: refactor
description: Run a behaviour-preserving refactoring pass on the tektite codebase, a given path, or the current diff. Reports findings ranked by severity across six lenses (deduplication, consistency, architecture, correctness, security, performance) before editing anything, then lands the fixes as small one-theme PRs. Use when asked to refactor, clean up, audit or tidy tektite code.
---

# Refactor tektite

Scope is the argument (a path or a diff such as `main...HEAD`), else the whole repo. Read AGENTS.md first; it is the rulebook.

## Rules

- Behaviour-preserving only. If a fix changes behaviour, report it as a finding and stop; do not fold it into a refactor PR.
- Never change `spec/` silently. If a scenario must change, say so explicitly in the report and the PR text.
- One lens or one theme per PR, branched from an up-to-date `main`, named `{type}/{short_desc}` in lowercase. Commits are a single Conventional Commit line, no body, no attribution. Do not merge.
- Before pushing run `make lint`, `make test` and `make build`. Tests must be green before and after.
- No inline comments. Only `why:` comments and exported docblocks in `parse` are allowed (commentlint).

## Process

1. Run the lens checks below over the scope. Do not edit.
2. Report findings ranked high, medium, low: `file:line`, lens, one-line problem, proposed fix, and whether it is safe (behaviour-preserving) or needs a decision.
3. Wait for the user to pick which to apply.
4. Group the chosen fixes into small PRs by lens or theme and apply one at a time, running the three make targets each time.

## Lenses

### 1. Deduplication
- Near-identical mappers: compare `internal/service/import_*.go` (`import_csv`, `import_jira_json`, `import_beads`, `import_mapping`) and move shared code into `import_helpers.go`.
- Parallel `cmd/tektite/cmd_*.go` files repeating flag parsing, output printing or error mapping; hoist into `app.go`.
- Copy-pasted tests: `grep -c "t.Run(" ` per file and look for repeated setup; extract helpers or table-drive, keeping each `t.Run` named after its spec scenario.
- Repeated string splitting, trimming or ID normalisation: `grep -rn "strings.TrimSpace\|strings.Split" internal`.

### 2. Consistency
- String literals where constants exist. Statuses are `open|active|blocked|review|done|dropped` (`event.Status*`), event types `set-status|claim|heartbeat|release|note|link|unlink|revision|attach|close|approve` (`parse.Event*`), node kinds `spec|plan|ticket|gap|evidence|decision|note` (`parse.Kind*`, `parse.DefaultKind`). Run `grep -rnE '"(open|active|blocked|review|done|dropped|set-status|claim|heartbeat|release|note|link|unlink|revision|attach|close|approve|spec|plan|ticket|gap|evidence|decision)"' internal cmd parse --include='*.go' | grep -v '_test.go' | grep -vE 'internal/event/fold.go|parse/(event|node|link).go'`. Re-read the constant blocks first so the pattern matches the current set, and ignore hits that are not statuses, kinds or event types (flag names, help text, JSON keys).
- Nil slices serialised to JSON: `grep -rn "var [a-zA-Z]* \[\]" internal cmd`; use `[]T{}` where the value is encoded.
- Errors: every failure from `internal/service` is a `*service.Error` with a `Kind`; check each Kind maps to exactly one exit code in `cmd/tektite` and that no `os.Exit` exists outside `main.go`.
- Flag names, short forms and help text identical across commands (`grep -n "fs\.\(String\|Bool\)" cmd/tektite/*.go`).
- Comment policy: `make lint` is the check.

### 3. Architectural integrity
- `parse` is pure: `grep -rn '"os"\|"io/ioutil"\|"net\|"os/exec"' parse` must be empty outside tests.
- `cmd/tektite` only parses flags and prints: look for loops, filtering, sorting or status logic in `cmd_*.go` that a second front end would need; move it to `internal/service` (writes) or `internal/view` (reads).
- `internal/mcp` calls `internal/service` and `internal/view`, never `cmd/tektite` or `exec` of the CLI: `grep -rn "os/exec\|cmd/tektite" internal/mcp`.
- Import cycles and reach-through: `go list -deps ./parse | grep tektite/internal` must be empty.

### 4. Correctness
- `scripts/spec-coverage.sh` lists scenarios no test names; report them, do not invent behaviour.
- Idempotence of write commands: running `claim`, `archive`, `approve`, `link`, `import` twice must not duplicate events or nodes; read the service code and add a test.
- Error paths: ignored errors (`grep -rn "_ = \|, _ :=" internal cmd | grep -v _test`), partial writes without rollback, `defer` closing files in loops.
- Empty, missing and duplicate inputs: empty store, unknown ref, self-link, cycle.

### 5. Security
- Path traversal: every path built from node refs, IDs, import fields or archive names must be cleaned and confined to the store root. `grep -rn "filepath.Join" internal`, then check the input is validated and `..`, absolute paths and symlinks are rejected (`store`, `import_*`, `archive`).
- Untrusted input: frontmatter, CSV and Jira/Linear/beads JSON are attacker-controlled. Check for size caps (`io.LimitReader`, max rows, max field length), depth limits and that parse failures are diagnostics, not panics (`grep -rn "panic(" parse internal`).
- Command execution: `os/exec` only in the `run` path, only when `--allow-run` is set on the MCP server; `grep -rn "exec.Command" --include=*.go .` and confirm no shell (`sh -c`) and no argument built from unvalidated input.
- File permissions: `grep -rnE "WriteFile|MkdirAll|OpenFile" internal`; files should be 0o644 or tighter, dirs 0o755 or tighter, no world-writable modes.

### 6. Performance
- Repeated full store loads inside a loop or per call: `grep -rn "store\.\(Load\|Open\|Read\)" internal cmd` and check each command loads once and passes the snapshot down.
- Quadratic scans: nested loops over nodes or events, `slices.Contains` or linear lookup inside a loop; replace with a map built once.
- Large imports: check rows are streamed or processed once, not re-read per row; add a benchmark only if you change a hot path.

## Output

Findings first, then (after approval) the list of PRs with branch, commit line and the lens it covers.
