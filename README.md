# tektite

Local-first work tracking for humans and AI agents, stored as markdown files in your git repo. The CLI is `tkt`.

Specs, plans, tickets, gaps and evidence are plain `.md` files under `.tektite/`. Agents pick up work with `tkt next`, get a prompt-ready briefing with `tkt context`, take an expiring lease with `tkt claim`, and prove they are done with `tkt attach-evidence`. Humans review in git. There is no server, no account and no database.

Use it when you want a coding agent (Claude Code, Codex, Cursor or similar) to work from a durable plan, not a chat history: the plan, the open gaps and the proof of each claim stay in the repo, versioned with the code.

## Install

```sh
brew install tektite-tracker/tap/tkt
```

Or download a binary for Linux, macOS or Windows from the [releases page](https://github.com/tektite-tracker/tektite/releases), or build from source with `make build` (Go 1.24+). Check it with `tkt --version`.

## Quickstart

```sh
tkt init
tkt new spec "Retry failed uploads"
tkt new ticket "Add backoff to the uploader" --parent retry-failed-uploads-3fa
tkt next --claim --actor agent:claude/session-1
tkt context add-backoff-to-the-uploader-9c1
tkt attach-evidence add-backoff-to-the-uploader-9c1 --run "go test ./..." --claim "backoff tests pass"
tkt set-status add-backoff-to-the-uploader-9c1 done --actor agent:claude/session-1
tkt check
```

References are a slug plus a short hash. The hash is the identity, so a renamed title still resolves. `tkt new` prints the reference to use. Add `--json` to any command for machine-readable output.

## For agents

Run these in order:

1. `tkt next --claim --actor agent:<model>/<session>` returns the best unblocked ticket and takes a 30 minute lease on it.
2. `tkt context <ref>` returns the node, its ancestor specs, open gaps, recent evidence and links, trimmed to a token budget (`--budget N`).
3. Do the work. `tkt heartbeat <ref>` extends the lease, `tkt note <ref> "text"` records progress, and `tkt new gap` records something left open.
4. `tkt attach-evidence <ref> --run "<command>"` runs a command and stores its output, exit code and commit as evidence. `tkt close-gap <ref>#<block> --evidence <ref>` closes a gap with it.
5. `tkt set-status <ref> done`. If a policy gate is not met the command exits 5 and says what is missing. An agent cannot override a gate.

Exit codes: 0 ok, 1 error, 2 usage, 3 not found, 4 conflict (someone else holds the lease), 5 policy violation.

### MCP server

`tkt mcp` serves the same verbs over the Model Context Protocol on stdio. Add it to your agent's MCP config:

```json
{
  "mcpServers": {
    "tektite": { "command": "tkt", "args": ["mcp"] }
  }
}
```

Tools: `tkt_show`, `tkt_ls`, `tkt_next`, `tkt_context`, `tkt_check`, `tkt_policy_explain`, `tkt_new`, `tkt_set_status`, `tkt_note`, `tkt_link`, `tkt_unlink`, `tkt_approve`, `tkt_revise`, `tkt_close_gap`, `tkt_claim`, `tkt_heartbeat`, `tkt_release`, `tkt_attach_evidence`, `tkt_rerun`. Running commands to record evidence is off unless you start the server with `--allow-run`.

## Commands

| Area | Commands |
| --- | --- |
| Set up | `init`, `new`, `fmt`, `schema` |
| Find work | `next`, `ls`, `show`, `context` |
| Work | `claim`, `heartbeat`, `release`, `set-status`, `note`, `link`, `unlink` |
| Prove | `attach-evidence`, `rerun`, `close-gap` |
| Plans | `revise`, `revisions`, `diff`, `approve` |
| Rules | `policy explain`, `check`, `check --stale` |
| Housekeeping | `gc`, `completion`, `mcp` |

Run `tkt <command> --help` for flags.

## How it works

- A **node** is a markdown file with frontmatter: `id` (ULID), `kind`, `title`, `parent`. `kind` is open (spec, plan, ticket, gap, evidence, decision, or your own). Nodes nest to any depth.
- Volatile state (status, claims, notes, links, approvals) lives in append-only **event** files next to the node, folded into current state when read. Two people or agents writing at once add files and never edit the same one, so git merges stay clean.
- **Evidence** is content-addressed under `.tektite/blobs/` with provenance: actor, time, commit, command and exit code.
- **Policy gates** in `.tektite/policy.md` block `claim` and `set-status done` until a plan is approved, evidence passes or gaps are closed. Only humans can `tkt approve` or pass `--override`.
- The parser is lenient. Any markdown file is a valid node, and `tkt check` reports problems without blocking reads.
- Actors are declared strings such as `human:jesus` or `agent:claude-opus/session-1`. Set one with `--actor` or `TKT_ACTOR`.

Layout:

```
.tektite/
  nodes/<slug-hash>.md
  events/<slug-hash>/<ulid>.md
  blobs/<hash>
  policy.md
  schemas/<kind>.md
```

## Library

The `parse` package turns bytes into nodes, events and diagnostics with no I/O, so other tools can reuse it: `github.com/tektite-tracker/tektite/parse`.

## Docs

- [`spec/`](spec/): Gherkin feature files, the source of truth for behaviour.
- [`docs/design.md`](docs/design.md): design decisions and why.
- [`AGENTS.md`](AGENTS.md): how to change this repo.

## Status

The parser, CLI, agent verbs, evidence, plan revisions, policy gates and MCP server are built. The local web UI is not.
