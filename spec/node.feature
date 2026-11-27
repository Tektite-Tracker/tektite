Feature: Nodes
  A node is a markdown file in .tektite/nodes/. It is the unit of work, knowledge,
  or proof. Specs, plans, tickets, gaps, and evidence are all nodes distinguished by kind.
  The parser is lenient: any markdown file is a valid node, and problems are reported
  by `tkt check` instead of blocking reads.

  Background:
    Given a repository with a .tektite/ directory

  Rule: A node file carries minimal, stable frontmatter

    Scenario: Parsing a complete node
      Given the file .tektite/nodes/auth-retry-3fa.md contains
        """
        ---
        id: 01HZ3K8Q5W2X9N4T7V6B1C0D3F
        kind: ticket
        title: Add retry on 429
        parent: auth-hardening-7c1
        ---
        The client should back off and retry when the API returns 429.
        """
      When the node is parsed
      Then its id is "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
      And its kind is "ticket"
      And its title is "Add retry on 429"
      And its parent is "auth-hardening-7c1"
      And its body is "The client should back off and retry when the API returns 429."

    Scenario: Optional fields are preserved but not required
      Given a node with frontmatter fields "id", "kind", "title", "parent", "labels", and "priority"
      When the node is parsed
      Then "labels" and "priority" are available as optional fields
      And a node without them is equally valid

    Scenario: A root node has no parent
      Given a node whose frontmatter has an empty "parent"
      When the node is parsed
      Then the node is a root of the tree
      And `tkt check` reports no warning for the missing parent

  Rule: Identity is the id, never the path or title

    Scenario: Renaming a title does not change identity or the file name
      Given a node "add-retry-on-429-3fa" with title "Add retry on 429"
      When the title is changed to "Add backoff on 429"
      Then the file is still named "add-retry-on-429-3fa.md"
      And its reference is still "add-retry-on-429-3fa"
      And `tkt show` displays the new title
      And no file is renamed by `tkt fmt` or any other command

    Scenario: The slug is derived from the title plus a short hash of the id
      Given a node titled "Add retry on 429" with id "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
      When the node is created with `tkt new`
      Then its reference has the form "add-retry-on-429-<hash>"
      And the file is named after that reference with a .md extension
      And the slug part is fixed at creation and never regenerated

    Scenario: A title with no ASCII letters or digits gets a hash-only reference
      Given a node titled "日本語" with id "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
      When the node is created with `tkt new`
      Then its reference is "d3f" with no slug part
      And titles are never transliterated

    Scenario: The hash is at least three characters and unique at creation
      Given an existing node whose hash is "3fa"
      When a new node would also get the hash "3fa"
      Then the new node's hash is lengthened until it is unique

    Scenario: A reference resolves by its hash suffix even if the slug part is stale
      Given a node "add-retry-on-429-3fa"
      When another node links to "retry-ticket-3fa"
      Then the link resolves to "add-retry-on-429-3fa"
      And `tkt check` notes that the slug part differs from the file name

    Scenario: A reference resolves by unique hash suffix
      Given nodes "auth-retry-3fa" and "auth-hardening-7c1"
      When a user runs `tkt show 3fa`
      Then the node "auth-retry-3fa" is shown

    Scenario: A reference with fewer than three hash characters is rejected
      Given nodes "auth-retry-3fa" and "auth-hardening-7c1"
      When a user runs `tkt show fa`
      Then the command fails because the hash is too short

    Scenario: Ids created in the same millisecond sort in creation order
      Given two nodes created in the same millisecond
      When their ids are compared
      Then the later node has the greater id

    Scenario: An ambiguous reference is rejected
      Given two nodes whose references end in the same hash prefix
      When a user runs `tkt show` with that prefix
      Then the command exits with code 2
      And the output lists the matching nodes

  Rule: Hierarchy lives in frontmatter, with unlimited depth

    Scenario: Nodes nest to any depth
      Given a spec, a plan whose parent is the spec, a ticket whose parent is the plan,
        and a sub-ticket whose parent is the ticket
      When the tree is built
      Then the sub-ticket's ancestors are the ticket, the plan, and the spec, in that order

    Scenario: Moving a node changes one line
      Given a ticket with parent "auth-hardening-7c1"
      When the ticket is moved under "session-rework-2ab"
      Then only the "parent" line in its frontmatter changes
      And no file is renamed

    Scenario: A cycle in parents is reported and not followed
      Given node A whose parent is B, and node B whose parent is A
      When the tree is built
      Then `tkt check` reports a cycle warning naming both nodes
      And reads of A and B still succeed

  Rule: Kind is an open string

    Scenario: Built-in kinds have default rules
      Given nodes of kind "spec", "plan", "ticket", "gap", "evidence", and "decision"
      When the nodes are parsed
      Then each kind is recognised with its default lifecycle and rendering

    Scenario: An unknown kind is accepted
      Given a node with kind "retro"
      When the node is parsed
      Then the node is valid
      And `tkt check` reports no error for the kind

    Scenario: A schema in .tektite/ can constrain a kind
      Given a schema for kind "ticket" that requires "estimate"
      And a ticket without "estimate"
      When `tkt check` runs
      Then a warning names the node and the missing field

  Rule: The parser repairs instead of failing

    Scenario: A file with no frontmatter is a valid node
      Given a markdown file in .tektite/nodes/ with no frontmatter
      When the node is parsed
      Then its kind defaults to "note"
      And its title is taken from the first heading, or the filename if there is none
      And `tkt check` reports that an id is missing

    Scenario: A missing id or kind on an imported note is a note
      Given a markdown file with no frontmatter, and a file whose frontmatter lacks an id and a kind
      When `tkt check` runs
      Then the missing id and the missing kind are reported with severity "note"
      And the exit code is 0 when nothing else is wrong

    Scenario: A node with complete frontmatter but no id is a warning
      Given a node whose frontmatter has a kind and a title but no id
      When the node is parsed
      Then a warning says the id is missing

    Scenario: A heading title keeps literal characters exactly as written
      Given a markdown file whose first heading is "snake_case <tag> & a < b *x*"
      When the node is parsed
      Then its title is that text exactly

    Scenario: A missing id is assigned by tkt fmt
      Given a node with no id
      When a user runs `tkt fmt`
      Then a new ULID is written into its frontmatter
      And the body is unchanged

    Scenario: Malformed frontmatter does not block reads
      Given a node whose frontmatter is not valid YAML
      When the node is parsed
      Then the whole file is treated as body
      And `tkt check` reports the YAML error with a line number

    Scenario: Malformed frontmatter takes its title from a heading or the file name
      Given a node whose frontmatter is not valid YAML and whose title line is "title: [unclosed"
      When the node is parsed
      Then its title is the first markdown heading after the frontmatter
      And without a heading its title is the file name without extension
      And no text inside the broken YAML is ever used as the title

    Scenario: An unterminated frontmatter block is reported
      Given a node that opens with "---" but has no closing "---" line
      When the node is parsed
      Then the whole file is treated as body
      And `tkt check` reports that the frontmatter is not closed

    Scenario: Duplicate frontmatter keys keep the last value with a warning
      Given a node whose frontmatter declares "title" twice
      When the node is parsed
      Then the last value wins and the key keeps its first position
      And `tkt check` reports a warning naming the repeated key

    Scenario: A non-string value in a core field is coerced or reported
      Given a node whose "id" is the number 42 and whose "title" is a list
      When the node is parsed
      Then the number is read as the string "42"
      And the list gives a warning and an empty value

    Scenario: Frontmatter and body can be split without rewriting the body
      Given a node file with frontmatter and a body
      When the file is split
      Then the frontmatter and the body are returned as written, minus any BOM and CRLF line endings
      And a file without a complete frontmatter block is returned whole as its body

    Scenario: Duplicate ids are reported and both nodes stay readable
      Given two files that declare the same id
      When `tkt check` runs
      Then an error names both files
      And `tkt ls` still lists both

    Scenario: Unknown frontmatter fields are preserved
      Given a node with a field "reviewed_by" that tektite does not know
      When `tkt fmt` runs
      Then the field is kept unchanged

  Rule: Bodies are plain markdown with addressable blocks

    Scenario: A checklist item can carry a stable block id
      Given a node body containing
        """
        - [ ] <!-- g:01HZ3KB2 --> missing retry on 429
        """
      When the node is parsed
      Then a block with id "01HZ3KB2" exists with text "missing retry on 429"
      And the block is unchecked

    Scenario: Blocks without ids are still parsed
      Given a node body containing "- [ ] handle timeouts"
      When the node is parsed
      Then the block exists with no id
      And `tkt fmt` offers to assign it one

    Scenario: A hand-written GAPS.md list is treated like gap blocks
      Given a node body with a section "## Gaps" containing checklist items
      When the node is parsed
      Then each item is available as a gap block

    Scenario: Wikilinks in prose are recognised as links
      Given a node body containing "see [[auth-retry-3fa]] for the retry logic"
      When the node is parsed
      Then the node has a link to "auth-retry-3fa"

    Scenario: A broken wikilink is a warning, not an error
      Given a node body containing "[[does-not-exist-000]]"
      When `tkt check` runs
      Then a warning names the node and the unresolved reference
      And reads of the node still succeed

    Scenario: The g marker is the only gap marker
      Given a node body containing "- [ ] <!-- g:01HZ3KB2 --> missing retry" and "- [ ] <!-- t:01HZ3KB3 --> plain task"
      When the node is parsed
      Then only the block with the "g" marker is a gap

    Scenario: A Gaps section ends at the next heading of the same or higher level
      Given a "## Gaps" section followed by "### Detail", then "## Notes"
      When the node is parsed
      Then checklist items under "## Gaps" and "### Detail" are gaps
      And checklist items under "## Notes" are not

    Scenario: Only checklist items are blocks
      Given a node body with a plain bullet "- not a task" and a checklist item "- [ ] a task"
      When the node is parsed
      Then only the checklist item is a block

    Scenario: The title falls back to the first heading of any level
      Given a markdown file with no title whose first heading is "### Deep heading"
      When the node is parsed
      Then its title is "Deep heading"

    Scenario: Wikilink aliases and anchors resolve to the target, and code is ignored
      Given a node body containing "[[a|label]]", "[[a#part]]", a repeated "[[a]]", and "[[b]]" in a code span
      When the node is parsed
      Then the node has exactly one link, to "a"

  Rule: Volatile state is not stored in the node file

    Scenario: Status is derived from events
      Given a node with no status field and an event "set-status: done"
      When the node's state is computed
      Then its status is "done"

    Scenario: A status field in frontmatter is ignored with a warning
      Given a node whose frontmatter contains "status: done" and no matching event
      When `tkt check` runs
      Then a warning says status belongs in events
      And the computed status is "open"
