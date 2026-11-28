Feature: Command line
  `tektite` is the interface for humans and agents. It prints human text by default and JSON with
  `--json`. Exit codes are stable so agents can branch on them without reading output.

  Rule: Exit codes are stable

    Scenario Outline: Exit codes
      When a command ends with <situation>
      Then it exits with code <code>

      Examples:
        | situation                          | code |
        | success                            | 0    |
        | an unexpected error                | 1    |
        | invalid usage or ambiguous input   | 2    |
        | a node or file that is not found   | 3    |
        | a conflict such as a live claim    | 4    |
        | a policy violation                 | 5    |

    Scenario: Errors go to stderr, data goes to stdout
      When a command fails
      Then the message is written to stderr
      And stdout stays empty or valid

  Rule: Output has two shapes

    Scenario: Human text is the default
      When a user runs `tektite show auth-retry-3fa`
      Then the output is readable text with the body and a summary of state

    Scenario: JSON output is stable
      When an agent runs `tektite show auth-retry-3fa --json`
      Then stdout is a single JSON document
      And its fields are documented and not renamed between minor versions

    Scenario: JSON errors
      When an agent runs `tektite show ghost-000 --json`
      Then stdout is a JSON object with "error", "code", and a hint
      And the exit code is 3

    Scenario: Unknown commands and flags are usage errors
      When a user runs `tektite frobnicate` or `tektite ls --nope`
      Then the command exits with code 2
      And the message is written to stderr

    Scenario: No colour or prompts when not a terminal
      When output is piped
      Then no colour codes are written
      And no command waits for interactive input

  Rule: The repository is discovered

    Scenario: The root is found by walking upwards
      Given the current directory is inside a project with a .tektite/ directory above it
      When a user runs `tektite ls`
      Then the nearest .tektite/ is used

    Scenario: No repository
      Given no .tektite/ directory exists above
      When a user runs `tektite ls`
      Then the command exits with code 3
      And the output suggests `tektite init`

    Scenario: Initialising
      When a user runs `tektite init`
      Then .tektite/ is created with nodes, events, blobs, and a config file
      And running it again changes nothing

    Scenario: An explicit root overrides discovery
      When a user runs `tektite --root ../other ls`
      Then the .tektite/ under ../other is used

  Rule: The actor is resolved without prompts

    Scenario: The actor comes from the environment first
      Given TKT_ACTOR is "agent:claude-opus/s-4f2a"
      When a verb appends an event
      Then the event's actor is "agent:claude-opus/s-4f2a"

    Scenario: A human actor falls back to git config
      Given TKT_ACTOR is unset and git user.name is "jesus"
      When a verb appends an event
      Then the actor is "human:jesus"

    Scenario: The actor flag wins
      Given TKT_ACTOR is "agent:a"
      When a user runs a verb with `--actor agent:b`
      Then the event's actor is "agent:b"

    Scenario: No actor available
      Given neither --actor, TKT_ACTOR nor git user.name is set
      When a verb appends an event
      Then the event is written with the actor "unknown"
      And a TKT-W006 warning explains how to set an actor
      And the command exits with code 0

  Rule: Creating and reading nodes

    Scenario: Creating a node
      When a user runs `tektite new ticket "Add retry on 429" --parent retry-plan-9de`
      Then a file is written under .tektite/nodes/ with a ULID and a slug-hash name
      And the new reference is printed

    Scenario: The body can come from stdin or an editor
      When a user runs `tektite new ticket "Add retry" --body -` with text on stdin
      Then the text becomes the body
      And without `--body` the user's editor is opened if the terminal is interactive

    Scenario: A new node stores its parent as a full reference
      When a user runs `tektite new ticket "Add retry" --parent 9de`
      Then the frontmatter parent is the full reference "retry-plan-9de"
      And a parent that matches no node exits with code 3 and creates nothing

    Scenario: Spawning a child
      When an agent runs `tektite spawn auth-retry-3fa "Handle Retry-After header"`
      Then a new node is created with parent "auth-retry-3fa"
      And its kind defaults to the parent's kind

    Scenario: Listing with filters
      When a user runs `tektite ls --kind ticket --status open --under auth-hardening-7c1`
      Then only matching nodes are listed

    Scenario: Listing what needs approval
      Given a policy "plans require human approval before work"
      And a plan with no approval at its current revision
      When a user runs `tektite ls --needs approval`
      Then only that plan is listed
      And an approved plan is not listed

    Scenario: Listing what needs an override
      Given a policy with a completion gate
      And a ticket in review that fails that gate
      When a user runs `tektite ls --needs override`
      Then only that ticket is listed
      And a ticket in review that passes every gate is not listed

    Scenario: Listing what needs review
      When a user runs `tektite ls --needs review`
      Then only nodes with status "review" are listed

    Scenario: An unknown needs value is a usage error
      When a user runs `tektite ls --needs lunch`
      Then the command exits with code 2
      And an empty result lists as [] with --json

    Scenario: Showing a tree
      When a user runs `tektite tree auth-hardening-7c1`
      Then the node and its descendants are printed indented to any depth

    Scenario: Searching text
      When a user runs `tektite search "429"`
      Then nodes whose title or body contain "429" are listed with a snippet

  Rule: Verbs append events

    Scenario: Setting a status
      When a user runs `tektite set-status auth-retry-3fa review --json`
      Then a set-status event is appended and its details are printed as JSON
      And `tektite show` and `tektite ls` show the status "review"

    Scenario: A verb on a node that does not exist
      When a user runs `tektite note ghost-000 "hello"`
      Then the command exits with code 3
      And no event is written

    Scenario: A link verb stores the full reference of the target
      When a user runs `tektite link auth-retry-3fa implements 9de`
      Then the event's target is the full reference "retry-plan-9de"

  Rule: Agents pick up work

    Scenario: Next returns the best available node
      Given several open, unblocked, unclaimed tickets under approved plans
      When an agent runs `tektite next`
      Then the highest priority one is returned
      And ties are broken by oldest first

    Scenario: Next skips claimed, blocked, and gated nodes
      Given a claimed ticket, a blocked ticket, and a ticket under an unapproved plan
      When an agent runs `tektite next`
      Then none of them is returned

    Scenario: Nothing to do
      Given no eligible nodes
      When an agent runs `tektite next`
      Then the command exits with code 3
      And the output says nothing is available

    Scenario: Next can claim in one step
      When an agent runs `tektite next --claim`
      Then the chosen node is claimed in the same command
      And a race with another agent is reported with code 4

  Rule: Claims are visible and tunable

    Scenario: A claim can set its own ttl
      When agent "agent:a" runs `tektite claim auth-retry-3fa --ttl 10m`
      Then the claim event has a ttl of 10 minutes
      And an invalid ttl exits with code 2

    Scenario: Show and ls display the current claim
      Given the ticket is claimed by "agent:a" with a live lease
      When a user runs `tektite show auth-retry-3fa` or `tektite ls`
      Then the holder and the lease expiry are listed
      And `--json` has a "claim" object with "holder", "expires_at" and "live"

    Scenario: Next ranks by the priority field then by age
      Given tickets with priority "low", "high" and a number 5
      When an agent runs `tektite next --limit 3 --json`
      Then they are listed by descending priority, where critical, high, medium and low rank 4 to 1
      And equal priorities are ordered by oldest id first

    Scenario: An expired claim does not hide a node from next
      Given a ticket whose only claim has expired
      When an agent runs `tektite next`
      Then the ticket is returned

  Rule: Writing is explicit about git

    Scenario: Commits are manual by default
      When a verb appends an event
      Then the file is written to the working tree
      And no git commit is created

    Scenario: Opt-in commit
      When a user runs a verb with `--commit`
      Then the new files are committed with a Conventional Commit message
      And only tektite's own files are included

    Scenario: Config can enable auto-commit
      Given "auto_commit: true" in .tektite/config
      When a verb appends an event
      Then a commit is created

  Rule: Maintenance commands

    Scenario: Checking the repository
      When a user runs `tektite check`
      Then warnings and errors are listed with file, line, and a short fix hint
      And the exit code is 0 when there are no errors, and 1 when errors exist

    Scenario: Notes alone do not fail the check
      Given only files with no frontmatter or partial frontmatter
      When a user runs `tektite check`
      Then each missing id or kind is listed as a note
      And the exit code is 0
      And `tektite check --json` shows "note" as the severity of each

    Scenario: Check output as JSON
      When an agent runs `tektite check --json`
      Then each problem is an object with "severity", "code", "path", "line", and "message"

    Scenario: Formatting
      When a user runs `tektite fmt`
      Then missing ids are assigned and frontmatter is normalised
      And bodies are not rewritten except to add block ids
      And `tektite fmt --dry-run` shows the changes without writing

    Scenario: Formatting is idempotent and skips broken files
      When a user runs `tektite fmt` twice
      Then the second run changes nothing
      And a file whose frontmatter is malformed is reported as skipped and left untouched

    Scenario: Formatting assigns ids to checklist blocks
      Given a node body with "- [ ] handle timeouts", a "## Gaps" section with "- [x] missing retry", and a block that already has an id
      When a user runs `tektite fmt`
      Then each checklist item without an id gets "<!-- t:ID -->", or "<!-- g:ID -->" when it is a gap
      And an id is the last 8 characters of a generated ULID
      And the text of each item and the rest of the body are unchanged
      And a block that already has an id keeps it
      And a second `tektite fmt` changes nothing

    Scenario: A dry run shows block ids without writing them
      Given a node body with a checklist item that has no id
      When a user runs `tektite fmt --dry-run`
      Then the change "assign block ids" is listed for the node
      And the file is not written

    Scenario: Formatting keeps the values, styles and comments of fields it does not own
      Given a node with a date "2024-01-05", quoted strings, a block scalar, a list, and a YAML comment
      When a user runs `tektite fmt`
      Then only the id, kind, title, and parent keys are added or moved to the top
      And every other field keeps its text, its quoting style, and its comments

    Scenario: A file whose values would change is skipped and left untouched
      Given a file whose reformatted frontmatter would parse to different field values or a different body
      When a user runs `tektite fmt`
      Then the file is reported as skipped
      And the file is unchanged

    Scenario: Init offers an agent onboarding snippet
      Given a terminal session and an AGENTS.md without the tektite snippet
      When a user runs `tektite init` and answers yes
      Then a short snippet that points agents at `tektite prime` is appended to AGENTS.md
      And the existing content is unchanged

    Scenario: The snippet goes to CLAUDE.md when there is no AGENTS.md
      Given a CLAUDE.md and no AGENTS.md
      When a user runs `tektite init --agent-snippet`
      Then the snippet is appended to CLAUDE.md

    Scenario: The snippet creates AGENTS.md when neither file exists
      Given neither AGENTS.md nor CLAUDE.md exists
      When a user runs `tektite init --agent-snippet`
      Then AGENTS.md is created with the snippet

    Scenario: The snippet is added only once
      Given AGENTS.md already holds the tektite snippet
      When a user runs `tektite init --agent-snippet`
      Then AGENTS.md is unchanged

    Scenario: The snippet can be skipped
      When a user runs `tektite init --no-agent-snippet`
      Then no agent file is written and no question is asked

    Scenario: Without a terminal init does not touch agent files
      Given no terminal and neither flag
      When a user runs `tektite init`
      Then no agent file is written

    Scenario: Priming an agent
      When an agent runs `tektite prime`
      Then the For agents steps and the exit codes are printed as prompt-ready text
      And `--json` prints the same steps and exit codes as a document

    Scenario: Serving the UI
      When a user runs `tektite ui`
      Then a local server starts on localhost
      And pages update live as files change

  Rule: Beads issues can be imported once

    Scenario: Importing beads issues
      Given a .beads/issues.jsonl with a task, a bug and an epic
      When a user runs `tektite migrate-beads`
      Then each issue becomes a node whose title and body come from the issue
      And the original id is kept in the beads_id field
      And the source file is not modified

    Scenario: Beads statuses map to tektite statuses
      Given beads issues that are open, in_progress, blocked, deferred and closed
      When a user runs `tektite migrate-beads`
      Then they are open, active, blocked, blocked and done
      And the deferred issue produces a warning

    Scenario: Deleted beads issues are imported as dropped
      Given a beads issue with status tombstone
      When a user runs `tektite migrate-beads`
      Then it becomes a node with status dropped
      And no warning is reported for the status
      And a blocks dependency that would make a live ticket blocked by it is not imported
      And a discovered-from dependency on it is kept
      And an unknown status still produces a warning

    Scenario: Beads bookkeeping fields are not imported
      Given a beads issue with _type, comment_count, dependency_count, dependent_count and updated_at
      When a user runs `tektite migrate-beads`
      Then none of those fields is written to the node frontmatter
      And created_by and owner are kept

    Scenario: Beads timestamps become the history
      Given a beads issue with created_at, closed_at and a comment with its own created_at
      When a user runs `tektite migrate-beads`
      Then the events written at creation are stamped with created_at
      And the done or dropped status event is stamped with closed_at
      And the comment event keeps its own created_at
      And created_at and closed_at are not written to the node frontmatter
      And running it again changes nothing

    Scenario: Beads issues without timestamps use the import time
      Given a beads issue with no created_at, closed_at or comment timestamps
      When a user runs `tektite migrate-beads`
      Then its events are stamped with the import time

    Scenario: Beads priorities and labels are kept
      Given a beads issue with priority 1 and labels
      When a user runs `tektite migrate-beads`
      Then the node has priority high and the same labels

    Scenario: Beads dependencies become parents and links
      Given beads issues related by parent-child, blocks and discovered-from dependencies
      When a user runs `tektite migrate-beads`
      Then the parent-child dependency sets the parent of the child
      And the blocks dependency becomes a blocks link on the blocker
      And the discovered-from dependency becomes a derived-from link

    Scenario: Unknown beads fields are kept as frontmatter
      Given a beads issue with a field tektite has no mapping for
      When a user runs `tektite migrate-beads`
      Then the field is written to the node frontmatter

    Scenario: Beads dependencies that cannot be mapped warn
      Given a beads issue with a related dependency and one on a missing issue
      When a user runs `tektite migrate-beads`
      Then a warning diagnostic is reported for each
      And the import still completes

    Scenario: A dry run writes nothing
      Given a .beads/issues.jsonl
      When a user runs `tektite migrate-beads --dry-run`
      Then the nodes that would be created are listed
      And no node or event file is written

    Scenario: Importing again changes nothing
      Given the issues were already imported
      When a user runs `tektite migrate-beads` again
      Then every issue is reported as skipped
      And no node or event file is written

    Scenario: A missing source file
      Given no .beads/issues.jsonl exists
      When a user runs `tektite migrate-beads`
      Then the command exits with code 3
