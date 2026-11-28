# Design decisions

Why tektite works the way it does. The Gherkin files in `spec/` are the source of truth for behaviour.

- Name: tektite. CLI binary: `tektite`.
- Language: Go.
- Plans are versioned as revisions.
- Commits are manual by default, with an opt-in auto-commit flag.
- Files live in the code repo under `.tektite/`.

- References are a slug from the title plus a short hash (e.g. `auth-retry-3fa`). The hash is the identity, so renames keep working.
- UI is embedded in the Go binary and served by `tektite ui` on localhost with live updates.
- Policy gates live in `.tektite/policy.md` (`version: 1`, `gates:` with `name`, `rule`, optional `kinds` and `labels`). Rules are `plan-approval`, `passing-evidence` and `no-open-gaps`. `claim` and `set-status` enforce them with exit 5, `tektite check` reports violations, `tektite policy explain <ref>` shows what applies, and only humans can `tektite approve` or `--override`.
- v1 is parser, CLI, and agent verbs. The UI comes after the format is proven with real agent use.

- Attachments are stored in the repo, content-addressed under `.tektite/blobs/<hash>`, with a size cap and a warning past it.
- Links are typed frontmatter fields plus inline `[[slug-hash]]` wikilinks.
- Actors are declared strings (`human:jesus`, `agent:claude-opus/<session>`), unsigned. Trust comes from git.
- `kind` is an open string. Built-ins (spec, plan, ticket, gap, evidence, decision) ship with default rules, and teams can add schemas in `.tektite/`.

- Shared lifecycle states: open, active, blocked, review, done, dropped. Kind schemas may restrict them.
- `tektite context --budget N` always includes the node, then fills by priority: ancestor specs, open gaps, recent evidence, links. Bodies shrink to summaries when over budget.
- One-way import (Jira, Linear, GitHub Issues) may come later. No outbound sync.
- Layout: `.tektite/nodes/<slug-hash>.md` with events in `.tektite/events/<slug-hash>/<ulid>.md`. Parent lives in frontmatter, so moves never rename files.

- Events are markdown files with frontmatter (`type`, `actor`, `at`, plus fields) and an optional free-text body.
- Claims are expiring leases (default 30 min TTL) renewed by heartbeat events. Once expired, anyone may claim.
- Plan revisions are `revision` events holding full snapshots of the plan body.
- Evidence provenance: actor, time, commit SHA, command, exit code, and the claim it is meant to prove.

- Node frontmatter requires only `id`, `kind`, `title`, `parent`. Everything else (`links`, `labels`, `priority`, `estimate`) is optional.
- The parser repairs silently and reports in `tektite check`: missing ids are assigned by `tektite fmt`, unknown kinds are accepted, broken links are warnings and never block reads.
- CLI prints human text by default and JSON with `--json`. Exit codes: 0 ok, 1 error, 2 usage, 3 not found, 4 conflict, 5 policy violation.
- Milestone 1: parser, `check`, `new`, `show`, `ls`, validated on real notes before agent verbs.

- The spec is written as Gherkin `.feature` files under `spec/`. They are a non-executable reference, not wired to a test runner.
- Go layout: `cmd/tektite` plus `internal/{node,event,store,check}` and a public `parse` package so the UI and other tools can reuse the parser.
- Fixtures: a small set of real notes plus hand-written synthetic cases, copied into `testdata/`.
- Dependencies: cobra for the CLI, `yaml.v3` for frontmatter, goldmark for body parsing (block IDs, wikilinks).

- File names are fixed at creation. A title change never renames a file. References resolve by hash suffix, so a stale slug still works.
- The event types are a closed list: set-status, claim, heartbeat, release, note, link, unlink, revision, attach, close, approve. An override is a field on set-status.
- Kind schemas live in `.tektite/schemas/<kind>.md` (`required`, `statuses`, `parent_kinds`, template body). They warn in `tektite check` and seed `tektite new`, and never block reads.
