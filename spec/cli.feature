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

    Scenario: Listing by label
      Given tickets labelled "net" and "auth" and a ticket with no labels
      When a user runs `tektite ls --label net --label ui`
      Then nodes that carry any of those labels are listed
      And combined with --kind, --status or --needs only nodes matching every flag are listed

    Scenario: Listing by priority
      Given tickets with priority "critical", "high" and "low" and a ticket with none
      When a user runs `tektite ls --priority critical --priority high`
      Then only the critical and high tickets are listed
      And an unknown priority value exits with code 2

    Scenario: Listing shows labels and priority
      Given a ticket with priority "high" and labels "net" and "auth"
      When a user runs `tektite ls --json`
      Then the item has "priority" "high" and "labels" ["net","auth"]
      And a node with neither has a null "priority" and an empty "labels" list
      And the plain output has a priority column that shows "-" when there is none

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

  Rule: Issues can be imported once from another tracker's export

    Scenario: Importing beads issues
      Given a .beads/issues.jsonl with a task, a bug and an epic
      When a user runs `tektite import beads`
      Then each issue becomes a node whose title and body come from the issue
      And the tracker is kept in the source field and the original id in the source_id field
      And the source file is not modified

    Scenario: Beads statuses map to tektite statuses
      Given beads issues that are open, in_progress, blocked, deferred and closed
      When a user runs `tektite import beads`
      Then they are open, active, blocked, blocked and done
      And the deferred issue produces a warning

    Scenario: Deleted beads issues are imported as dropped
      Given a beads issue with status tombstone
      When a user runs `tektite import beads`
      Then it becomes a node with status dropped
      And no warning is reported for the status
      And a blocks dependency that would make a live ticket blocked by it is not imported
      And a discovered-from dependency on it is kept
      And an unknown status still produces a warning

    Scenario: Beads bookkeeping fields are not imported
      Given a beads issue with _type, comment_count, dependency_count, dependent_count and updated_at
      When a user runs `tektite import beads`
      Then none of those fields is written to the node frontmatter
      And created_by and owner are kept

    Scenario: Beads timestamps become the history
      Given a beads issue with created_at, closed_at and a comment with its own created_at
      When a user runs `tektite import beads`
      Then the events written at creation are stamped with created_at
      And the done or dropped status event is stamped with closed_at
      And the comment event keeps its own created_at
      And created_at and closed_at are not written to the node frontmatter
      And running it again changes nothing

    Scenario: Beads issues without timestamps use the import time
      Given a beads issue with no created_at, closed_at or comment timestamps
      When a user runs `tektite import beads`
      Then its events are stamped with the import time

    Scenario: Beads priorities and labels are kept
      Given a beads issue with priority 1 and labels
      When a user runs `tektite import beads`
      Then the node has priority high and the same labels

    Scenario: Beads dependencies become parents and links
      Given beads issues related by parent-child, blocks and discovered-from dependencies
      When a user runs `tektite import beads`
      Then the parent-child dependency sets the parent of the child
      And the blocks dependency becomes a blocks link on the blocker
      And the discovered-from dependency becomes a derived-from link

    Scenario: Unknown beads fields are kept as frontmatter
      Given a beads issue with a field tektite has no mapping for
      When a user runs `tektite import beads`
      Then the field is written to the node frontmatter

    Scenario: Beads dependencies that cannot be mapped warn
      Given a beads issue with a related dependency and one on a missing issue
      When a user runs `tektite import beads`
      Then a warning diagnostic is reported for each
      And the import still completes

    Scenario: A dry run writes nothing
      Given a .beads/issues.jsonl
      When a user runs `tektite import beads --dry-run`
      Then the nodes that would be created are listed
      And no node or event file is written

    Scenario: Importing again changes nothing
      Given the issues were already imported
      When a user runs `tektite import beads` again
      Then every issue is reported as skipped
      And no node or event file is written

    Scenario: A missing source file
      Given no .beads/issues.jsonl exists
      When a user runs `tektite import beads`
      Then the command exits with code 3

    Scenario: Nodes imported before the source field existed are recognised
      Given a node with a beads_id field and no source field
      When a user runs `tektite import beads`
      Then the issue with that id is reported as skipped
      And no duplicate node is written

  Rule: Status, priority and type mappings are confirmed once

    Scenario: Values are mapped by category, timestamps and names
      Given an export whose statuses carry a category column, timestamp columns or only a name
      When the import proposes a mapping
      Then the category column decides first, the timestamp columns next and name keywords last
      And a name keyword only refines a value inside the category the evidence gave

    Scenario: Only ambiguous values are asked about
      Given an export with one status that no evidence or keyword explains
      When a user runs the import in a terminal
      Then only that value is asked about
      And the answer is checked against the allowed targets

    Scenario: Accepting the proposal with --yes
      Given an export with an ambiguous status that has a best guess
      When a user runs the import with --yes
      Then the guess is used without asking

    Scenario: The confirmed mapping is saved and reused
      Given the import ran once and confirmed a mapping
      Then the mapping is written to .tektite/import/<source>.yaml
      And a later run reuses it without asking

    Scenario: A new value in a later export is asked about alone
      Given a saved mapping and an export with one status it does not cover
      When a user runs the import
      Then only the new value is asked about
      And it is added to the saved mapping

    Scenario: A mapping file can be supplied
      Given a mapping file
      When a user runs the import with --map and that file
      Then the file is used without asking and nothing is saved

    Scenario: A dry run prints the proposed mapping
      Given an export and no saved mapping
      When a user runs the import with --dry-run
      Then the proposed mapping and the nodes that would be created are printed
      And no mapping, node or event file is written

    Scenario: No terminal and no saved mapping fails
      Given an export with an ambiguous status and no saved mapping
      When a user runs the import without a terminal and without --yes
      Then the command exits with code 2
      And the message names the unmapped values

    Scenario: Unknown values are never mapped to open
      Given an export with a status that has no evidence, keyword or guess
      When a user runs the import with --yes
      Then the command exits with code 2
      And no node is written

  Rule: Linear issues can be imported from a CSV export

    Scenario: A source needs an export file
      Given a user runs `tektite import linear` without --from
      Then the command exits with code 2

    Scenario: An export of the wrong shape is refused
      Given a file that is not a Linear CSV export
      When a user runs `tektite import linear --from` that file
      Then the command exits with code 2
      And the message names the missing column

    Scenario: Importing Linear issues
      Given a Linear CSV with issues that have a title, description, labels, priority and assignee
      When a user runs `tektite import linear --from` that file
      Then each issue becomes a ticket whose title and body come from the issue
      And the tracker is kept in the source field and the issue id in the source_id field
      And the labels, assignee and priority are kept

    Scenario: Linear statuses are mapped by workflow state
      Given Linear issues whose statuses have custom names and Started, Completed and Canceled times
      When a user runs `tektite import linear --from` that file
      Then an issue with a Completed time becomes done and one with a Canceled time becomes dropped
      And an issue with a Started time only becomes active
      And a status with none of these is mapped by its name or asked about

    Scenario: Linear parent issues become parents
      Given Linear issues with a Parent issue column
      When a user runs `tektite import linear --from` that file
      Then the parent issue is created first and the child gets it as parent
      And a parent that is not in the export produces a warning

    Scenario: Linear relations become typed links
      Given Linear issues with Blocked by, Related to and Duplicate of columns
      When a user runs `tektite import linear --from` that file
      Then a Blocked by relation becomes a blocks link on the blocker
      And a Related to relation becomes a single relates-to link even when both issues list it
      And a Duplicate of relation becomes a supersedes link on the original

    Scenario: Linear timestamps become the history
      Given a Linear issue with Created, Started and Completed times
      When a user runs `tektite import linear --from` that file
      Then the node is stamped with Created
      And the active status event is stamped with Started
      And the done status event is stamped with Completed

    Scenario: Linear columns without a mapping are kept as frontmatter
      Given a Linear CSV with Team, Estimate and Project columns
      When a user runs `tektite import linear --from` that file
      Then each non-empty one is written to the node frontmatter with a lowercase key
      And Updated is not written

    Scenario: Importing Linear issues again changes nothing
      Given the Linear issues were already imported
      When a user runs `tektite import linear --from` that file again
      Then every issue is reported as skipped



  Rule: Jira issues can be imported from a CSV export

    Scenario: An export of the wrong shape is refused by jira
      Given a file that is not a Jira CSV export
      When a user runs `tektite import jira --from` that file
      Then the command exits with code 2
      And the message names the missing column

    Scenario: Importing Jira issues
      Given a Jira CSV with issues that have a summary, description, labels, priority, assignee and type
      When a user runs `tektite import jira --from` that file
      Then each issue becomes a node whose title and body come from the issue
      And the tracker is kept in the source field and the issue key in the source_id field
      And labels from repeated Labels columns are all kept

    Scenario: Jira statuses are mapped by status category
      Given Jira issues with custom status names and a Status Category column
      When a user runs `tektite import jira --from` that file
      Then To Do, In Progress and Done categories become open, active and done
      And a name such as review or won't do refines the status inside its category

    Scenario: Jira epics become plans
      Given a Jira epic with child issues that name it in Parent or Parent key
      When a user runs `tektite import jira --from` that file
      Then the epic becomes a plan and each child gets it as parent
      And a parent given by issue id is resolved to its key

    Scenario: Jira issue links become typed links
      Given Jira issues with Outward issue link columns for Blocks, Duplicate and Relates
      When a user runs `tektite import jira --from` that file
      Then Blocks becomes a blocks link, Duplicate a supersedes link and Relates a relates-to link
      And a link type tektite does not know becomes a link of that name

    Scenario: Jira dates are read in the export's format
      Given Jira dates written as 27/Mar/24 3:12 PM, as 28-03-2024 14:44 or as 04-08-2025 14:44
      When a user runs `tektite import jira --from` that file
      Then the first two are read as written and the node is stamped with them
      And a date that could be day first or month first is read as day first with a warning

    Scenario: Jira resolved times and comments become the history
      Given a Jira issue with a Resolved time and a comment with its own time
      When a user runs `tektite import jira --from` that file
      Then the done or dropped status event is stamped with Resolved
      And the comment event keeps its own time

    Scenario: Jira columns without a mapping are kept as frontmatter
      Given a Jira CSV with Sprint, Reporter and Custom field columns
      When a user runs `tektite import jira --from` that file
      Then each non-empty one is written to the node frontmatter with a lowercase key
      And Updated and the sigma estimate columns are not written

    Scenario: Importing a Jira JSON export
      Given a JSON file with an issues list in the shape of the Jira search API, as an MCP server or a script can save it
      When a user runs `tektite import jira --from` that file
      Then the format is detected from the content, not the file name
      And the issues, statuses by status category, epic parents, issue links, resolution time and comments are imported as they are from a CSV

    Scenario: Jira rich text becomes plain text
      Given a Jira JSON issue whose description and comments are in Atlassian document format
      When a user runs `tektite import jira --from` that file
      Then the node body and the comment text hold the plain text with paragraphs and list items kept

    Scenario: A JSON file that is not a Jira export is refused
      Given a JSON file with neither an issues list nor issue objects with a key
      When a user runs `tektite import jira --from` that file
      Then the command exits with code 2

    Scenario: A won't do resolution drops a done status
      Given a Jira status in the Done category whose issues were all resolved as Won't Do or Duplicate
      When a user runs `tektite import jira --from` that file
      Then the status is proposed as dropped, whatever its name

    Scenario: Spanish status names are recognised by keyword
      Given statuses named in Spanish such as Por hacer, En curso, Hecho and Descartado
      When the import proposes a mapping
      Then they are proposed as open, active, done and dropped

    Scenario: Jira rich text keeps bold, italic, code and links
      Given a Jira JSON description with strong, em, code and link marks and a heading
      When a user runs `tektite import jira --from` that file
      Then the node body holds the same emphasis, code, links and heading as Markdown

    Scenario: A won't do resolution drops a done status
      Given a Jira status in the Done category whose issues were all resolved as Won't Do or Duplicate
      When a user runs `tektite import jira --from` that file
      Then the status is proposed as dropped, whatever its name

    Scenario: Spanish status names are recognised by keyword
      Given statuses named in Spanish such as Por hacer, En curso, Hecho and Descartado
      When the import proposes a mapping
      Then they are proposed as open, active, done and dropped

    Scenario: Jira rich text keeps bold, italic, code and links
      Given a Jira JSON description with strong, em, code and link marks and a heading
      When a user runs `tektite import jira --from` that file
      Then the node body holds the same emphasis, code, links and heading as Markdown

    Scenario: Importing a Linear JSON export
      Given a JSON file with an issues list in the shape the Linear MCP server returns, as an MCP server or a script can save it
      When a user runs `tektite import linear --from` that file
      Then the format is detected from the content, not the file name
      And the status type decides the status before timestamps and names
      And parent ids, labels, priorities, times and the blocks, blockedBy, relatedTo and duplicateOf relations are imported

    Scenario: A truncated Linear description warns
      Given a Linear JSON issue whose description ends with the truncation marker
      When a user runs `tektite import linear --from` that file
      Then the marker is not written to the node body
      And a warning says the description was truncated

    Scenario: Linear image tags are replaced by a placeholder
      Given a Linear JSON description with a linear-image tag that holds a signed URL
      When a user runs `tektite import linear --from` that file
      Then the node body holds a placeholder and no signed URL

    Scenario: A Linear JSON file that is not an export is refused
      Given a JSON file with neither an issues list nor an issue object with an id
      When a user runs `tektite import linear --from` that file
      Then the command exits with code 2
