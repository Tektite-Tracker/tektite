Feature: Kind schemas
  A schema describes the rules for one kind of node. Schemas live in .tektite/schemas/<kind>.md
  as markdown with frontmatter. They are optional: built-in kinds ship with embedded defaults,
  custom kinds work without a schema, and a local schema overrides defaults key by key.
  Schemas produce warnings in `tektite check` and defaults in `tektite new`. They never block reads.

  Background:
    Given a repository with a .tektite/ directory

  Rule: A schema is a markdown file named after its kind

    Scenario: Loading a schema
      Given the file .tektite/schemas/ticket.md contains
        """
        ---
        kind: ticket
        required: [estimate]
        statuses: [open, active, blocked, review, done, dropped]
        parent_kinds: [plan, ticket]
        ---
        ## Template
        ### Context
        ### Acceptance criteria
        """
      When schemas are loaded
      Then the schema for "ticket" requires the field "estimate"
      And allows the given statuses and parent kinds
      And has a body template

    Scenario: The file name must match the kind
      Given the file .tektite/schemas/ticket.md declares "kind: plan"
      When `tektite check` runs
      Then a warning names the mismatch
      And the schema is applied to "plan"

    Scenario: A malformed schema is reported and ignored
      Given a schema with invalid frontmatter
      When `tektite check` runs
      Then an error names the file and line
      And the built-in defaults for that kind are used

    Scenario: Unknown keys in a schema are preserved
      Given a schema with a key "owner"
      When `tektite fmt` runs
      Then the key is kept unchanged

  Rule: Built-in kinds ship with defaults

    Scenario: Defaults exist without any schema file
      Given no .tektite/schemas/ directory
      When a node of kind "plan" is validated
      Then the embedded default schema for "plan" applies

    Scenario: A local schema overrides defaults key by key
      Given the default "ticket" schema allows all shared statuses
      And a local schema for "ticket" that sets only "required: [estimate]"
      When a ticket is validated
      Then "estimate" is required
      And the statuses still come from the default

    Scenario: The built-in kinds
      Given the kinds spec, plan, ticket, gap, evidence, and decision
      When the defaults are listed with `tektite schema ls`
      Then each kind appears with its source "built-in" or "local"

    Scenario: Custom kinds need no schema
      Given a node of kind "retro" and no schema for it
      When `tektite check` runs
      Then no problem is reported

  Rule: Schemas constrain with warnings

    Scenario: A missing required field
      Given a schema requiring "estimate" on tickets
      And a ticket without "estimate"
      When `tektite check` runs
      Then a warning names the node and the field

    Scenario: A disallowed status
      Given a schema for "gap" allowing only open and done
      And a gap whose folded status is "review"
      When `tektite check` runs
      Then a warning names the node and the status

    Scenario: A disallowed parent kind
      Given a schema for "ticket" with parent_kinds [plan, ticket]
      And a ticket whose parent is a spec
      When `tektite check` runs
      Then a warning names the node, its kind, and the parent's kind

    Scenario: A root node ignores parent_kinds
      Given a schema with parent_kinds [plan]
      And a node of that kind with no parent
      When `tektite check` runs
      Then no parent-kind warning is reported

    Scenario: Schema warnings are not errors
      Given any schema violation
      When `tektite check` runs
      Then the exit code is 0 unless other errors exist

  Rule: Schemas provide defaults to creation

    Scenario: The template seeds a new body
      Given a schema with a "## Template" section
      When a user runs `tektite new ticket "Add retry"` with no body
      Then the new node's body starts with the template text

    Scenario: Explicit body overrides the template
      When a user runs `tektite new ticket "Add retry" --body "short"`
      Then the body is "short"

    Scenario: A required field is requested at creation
      Given a schema requiring "estimate"
      When an agent runs `tektite new ticket "Add retry"` without "--set estimate=3"
      Then the node is still created
      And the output says "estimate" is required by the schema

    Scenario: Setting fields at creation
      When an agent runs `tektite new ticket "Add retry" --set estimate=3`
      Then the node's frontmatter contains "estimate: 3"

  Rule: Schemas are shared through git

    Scenario: Schemas are ordinary tracked files
      When a user commits .tektite/schemas/
      Then collaborators who pull get the same rules
      And no other registration step is needed
