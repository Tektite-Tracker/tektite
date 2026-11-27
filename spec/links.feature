Feature: Links
  Nodes form a tree through "parent" and a graph through typed links. Links are written as
  frontmatter fields, as inline [[wikilinks]] in prose, or added later by link events.
  Links never block reads: a broken link is a warning.

  Background:
    Given a repository with a .tektite/ directory

  Rule: Typed links live in frontmatter

    Scenario: Declaring typed links
      Given a ticket "auth-retry-3fa" whose frontmatter contains
        """
        implements: [retry-plan-9de]
        blocks: [release-notes-1ab]
        """
      When the node is parsed
      Then it has an "implements" link to "retry-plan-9de"
      And it has a "blocks" link to "release-notes-1ab"

    Scenario: The built-in link types
      Given links of type "implements", "blocks", "evidences", "supersedes", and "derived-from"
      When the links are parsed
      Then each type is recognised with its inverse
      And the inverses are "implemented-by", "blocked-by", "evidenced-by", "superseded-by", and "source-of"

    Scenario: An unknown link type is accepted
      Given a link of type "inspired-by"
      When the node is parsed
      Then the link is kept
      And `tkt check` reports no error

    Scenario: Link targets accept any unambiguous reference
      Given a link to "3fa" and a node "auth-retry-3fa"
      When the link is resolved
      Then it resolves to "auth-retry-3fa"

  Rule: Wikilinks in prose are untyped links

    Scenario: A wikilink is a "mentions" link
      Given a body containing "see [[auth-retry-3fa]]"
      When the node is parsed
      Then it has a "mentions" link to "auth-retry-3fa"

    Scenario: A typed wikilink uses a prefix
      Given a body containing "implements:: [[retry-plan-9de]]"
      When the node is parsed
      Then it has an "implements" link to "retry-plan-9de"

    Scenario: A wikilink with an alias keeps the target
      Given a body containing "[[auth-retry-3fa|the retry ticket]]"
      When the node is parsed
      Then the link target is "auth-retry-3fa"
      And the display text is "the retry ticket"

    Scenario: A wikilink to a block targets that block
      Given a body containing "[[auth-retry-3fa#01HZ3KB2]]"
      When the node is parsed
      Then the link targets the block "01HZ3KB2" in "auth-retry-3fa"

    Scenario: Wikilinks inside code spans and fences are ignored
      Given a body with "[[not-a-link]]" inside a fenced code block
      When the node is parsed
      Then the node has no link to "not-a-link"

  Rule: Links can be added and removed by events

    Scenario: The folded link set is frontmatter plus events
      Given a node with an "implements" link in frontmatter
      And a link event adding "blocks" and an unlink event removing "implements"
      When the node's state is computed
      Then the node has the "blocks" link
      And the node has no "implements" link

  Rule: Backlinks are computed, never stored

    Scenario: A link has an inverse on its target
      Given ticket A with "implements: [plan-p]"
      When the graph is built
      Then plan P shows an "implemented-by" link to A

    Scenario: Backlinks require no edit to the target
      Given ticket A links to plan P
      When the graph is built
      Then the file of plan P is unchanged

  Rule: Blocking affects what is workable

    Scenario: A blocked node is not offered by `tkt next`
      Given ticket A is blocked-by ticket B, and B is not done
      When an agent runs `tkt next`
      Then A is not returned

    Scenario: Finishing the blocker releases the node
      Given ticket A is blocked-by ticket B
      When B's status becomes "done"
      Then A is eligible for `tkt next` again

    Scenario: A dropped blocker does not hold the node forever
      Given ticket A is blocked-by ticket B, and B is "dropped"
      When `tkt next` runs
      Then A is eligible
      And `tkt check` notes the dropped blocker

    Scenario: Cyclic blocking is reported
      Given A blocks B and B blocks A
      When `tkt check` runs
      Then a warning names the cycle
      And neither node is returned by `tkt next`

  Rule: Supersession retires a node without deleting it

    Scenario: A superseded node is hidden by default
      Given plan P2 supersedes plan P1
      When a user runs `tkt ls`
      Then P1 is not listed
      And `tkt ls --all` lists both

    Scenario: A superseded node keeps its history
      Given plan P2 supersedes plan P1
      When a user runs `tkt show` on P1
      Then P1 shows its events and a pointer to P2

  Rule: Broken links are warnings

    Scenario: A link to a missing node
      Given a link to "ghost-000" that matches no node
      When `tkt check` runs
      Then a warning names the node, the link type, and the target
      And reads of the node still succeed

    Scenario: A link to oneself
      Given a node linking to itself
      When `tkt check` runs
      Then a warning is reported for the self-link
