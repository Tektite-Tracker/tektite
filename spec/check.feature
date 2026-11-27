Feature: Check
  `tkt check` aggregates what the parser, the store, and the schemas report into one list of
  problems. It reads the whole repository, never changes it, and reports policy gate violations and a
  malformed policy file as described in policy.feature.

  Background:
    Given a repository with a .tektite/ directory

  Rule: Every problem is located and ranked

    Scenario: A clean repository has no problems
      Given nodes that parse cleanly, resolve every reference, and satisfy their schemas
      When `tkt check` runs
      Then no problem is reported

    Scenario: Problems are ordered and counted
      Given problems in several files
      When `tkt check` runs
      Then problems are ordered by path, line, and code
      And the report counts errors, warnings, and notes
      And running it again gives the same report

  Rule: References are checked wherever they are written

    Scenario: A parent that matches no node is a warning
      Given a ticket whose parent is "ghost-000"
      When `tkt check` runs
      Then a warning names the ticket and the unresolved parent
      And the ticket is still listed as a root

    Scenario: A link added by event is checked like any other
      Given a link event on a ticket whose target matches no node
      When `tkt check` runs
      Then a warning names the node, the link type, and the target

    Scenario: An ambiguous or too short link target is a warning
      Given a link to "fa" and a link to a hash that matches two nodes
      When `tkt check` runs
      Then a warning says the first hash is too short
      And a warning lists the nodes matched by the second
