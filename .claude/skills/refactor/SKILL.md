---
name: refactor
description: Run a behaviour-preserving refactoring pass on the tektite codebase, a given path, or the current diff. Reports findings ranked by severity across six lenses (deduplication, consistency, architecture, correctness, security, performance) before editing anything, then lands the fixes as small one-theme PRs. Use when asked to refactor, clean up, audit or tidy tektite code.
---

# Refactor tektite

Scope is the argument (a path or a diff such as `main...HEAD`), else the whole repo. Read AGENTS.md first; it is the rulebook for layout, conventions, comments, tests and commits.

## Rules

- Behaviour-preserving only. If a fix changes behaviour, report it as a finding and stop; do not fold it into a refactor PR.
- Never change `spec/` silently. If a scenario must change, say so in the report and the PR text.
- One lens or one theme per PR, small enough to review at a glance. Follow AGENTS.md for branches, commits and CI. Do not merge.
- Tests must be green before and after.

## Process

1. Read the scope through each lens below. Do not edit.
2. Report findings ranked high, medium, low: location, lens, the problem, the proposed fix, and whether it is safe or needs a decision.
3. Wait for the user to pick which to apply.
4. Group the chosen fixes into small PRs and apply them one at a time.

## Lenses

- **Deduplication.** Repeated helpers, near-identical mappers or commands, copy-pasted tests. Merge only when the copies really mean the same thing; leave look-alikes that differ on purpose.
- **Consistency.** Same names, constants, error kinds, exit codes, flags and output shapes for the same concepts across the codebase. Follow the conventions in AGENTS.md.
- **Architectural integrity.** The layering in AGENTS.md holds: the public package stays pure, front ends stay thin, shared logic lives where a second front end can reuse it, and dependencies point the right way.
- **Correctness.** Edge cases, error paths, partial writes, and write commands that are not safe to run twice. Spec scenarios with no test are reported, not invented.
- **Security.** Anything built from untrusted input (refs, frontmatter, imported files, flags): path confinement, size and depth limits, failures as diagnostics rather than panics, command execution, file permissions.
- **Performance.** Repeated loads of the same data, quadratic scans, work that scales badly with large stores or imports. Measure before changing a hot path.

## Output

Findings first, then, after approval, the list of PRs with branch, commit line and the lens each covers.
