Feature: Command line
  `tkt` is the interface for humans and agents. It prints human text by default and JSON with
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
      When a user runs `tkt show auth-retry-3fa`
      Then the output is readable text with the body and a summary of state

    Scenario: JSON output is stable
      When an agent runs `tkt show auth-retry-3fa --json`
      Then stdout is a single JSON document
      And its fields are documented and not renamed between minor versions

    Scenario: JSON errors
      When an agent runs `tkt show ghost-000 --json`
      Then stdout is a JSON object with "error", "code", and a hint
      And the exit code is 3

    Scenario: Unknown commands and flags are usage errors
      When a user runs `tkt frobnicate` or `tkt ls --nope`
      Then the command exits with code 2
      And the message is written to stderr

    Scenario: No colour or prompts when not a terminal
      When output is piped
      Then no colour codes are written
      And no command waits for interactive input

  Rule: The repository is discovered

    Scenario: The root is found by walking upwards
      Given the current directory is inside a project with a .tektite/ directory above it
      When a user runs `tkt ls`
      Then the nearest .tektite/ is used

    Scenario: No repository
      Given no .tektite/ directory exists above
      When a user runs `tkt ls`
      Then the command exits with code 3
      And the output suggests `tkt init`

    Scenario: Initialising
      When a user runs `tkt init`
      Then .tektite/ is created with nodes, events, blobs, and a config file
      And running it again changes nothing

    Scenario: An explicit root overrides discovery
      When a user runs `tkt --root ../other ls`
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

    Scenario: No actor available
      Given neither TKT_ACTOR nor git user.name is set
      When a verb appends an event
      Then the command exits with code 2
      And the output explains how to set an actor

  Rule: Creating and reading nodes

    Scenario: Creating a node
      When a user runs `tkt new ticket "Add retry on 429" --parent retry-plan-9de`
      Then a file is written under .tektite/nodes/ with a ULID and a slug-hash name
      And the new reference is printed

    Scenario: The body can come from stdin or an editor
      When a user runs `tkt new ticket "Add retry" --body -` with text on stdin
      Then the text becomes the body
      And without `--body` the user's editor is opened if the terminal is interactive

    Scenario: A new node stores its parent as a full reference
      When a user runs `tkt new ticket "Add retry" --parent 9de`
      Then the frontmatter parent is the full reference "retry-plan-9de"
      And a parent that matches no node exits with code 3 and creates nothing

    Scenario: Spawning a child
      When an agent runs `tkt spawn auth-retry-3fa "Handle Retry-After header"`
      Then a new node is created with parent "auth-retry-3fa"
      And its kind defaults to the parent's kind

    Scenario: Listing with filters
      When a user runs `tkt ls --kind ticket --status open --under auth-hardening-7c1`
      Then only matching nodes are listed

    Scenario: Showing a tree
      When a user runs `tkt tree auth-hardening-7c1`
      Then the node and its descendants are printed indented to any depth

    Scenario: Searching text
      When a user runs `tkt search "429"`
      Then nodes whose title or body contain "429" are listed with a snippet

  Rule: Agents pick up work

    Scenario: Next returns the best available node
      Given several open, unblocked, unclaimed tickets under approved plans
      When an agent runs `tkt next`
      Then the highest priority one is returned
      And ties are broken by oldest first

    Scenario: Next skips claimed, blocked, and gated nodes
      Given a claimed ticket, a blocked ticket, and a ticket under an unapproved plan
      When an agent runs `tkt next`
      Then none of them is returned

    Scenario: Nothing to do
      Given no eligible nodes
      When an agent runs `tkt next`
      Then the command exits with code 3
      And the output says nothing is available

    Scenario: Next can claim in one step
      When an agent runs `tkt next --claim`
      Then the chosen node is claimed in the same command
      And a race with another agent is reported with code 4

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
      When a user runs `tkt check`
      Then warnings and errors are listed with file, line, and a short fix hint
      And the exit code is 0 when there are no errors, and 1 when errors exist

    Scenario: Check output as JSON
      When an agent runs `tkt check --json`
      Then each problem is an object with "severity", "code", "path", "line", and "message"

    Scenario: Formatting
      When a user runs `tkt fmt`
      Then missing ids are assigned and frontmatter is normalised
      And bodies are not rewritten except to add block ids
      And `tkt fmt --dry-run` shows the changes without writing

    Scenario: Formatting is idempotent and skips broken files
      When a user runs `tkt fmt` twice
      Then the second run changes nothing
      And a file whose frontmatter is malformed is reported as skipped and left untouched

    Scenario: Teaching agents the workflow
      When a user runs `tkt init-agent`
      Then a snippet is written for CLAUDE.md or AGENTS.md describing the verbs and exit codes

    Scenario: Serving the UI
      When a user runs `tkt ui`
      Then a local server starts on localhost
      And pages update live as files change
