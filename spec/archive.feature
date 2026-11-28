Feature: Archive
  Done and dropped nodes can be moved out of the active set into .tektite/archive/ with their events
  and blobs. An archive move is a plain file move, so git history follows it and merges stay clean.
  Archived nodes still resolve by reference and in `tektite show`, and links to them are not broken,
  but `ls`, `next`, `context` and `check` leave them out unless asked with --archived or --all.

  Background:
    Given a repository with a .tektite/ directory

  Rule: Only finished work that nothing depends on is archived

    Scenario: Archiving moves a closed node with its events and blobs
      Given a done ticket with events and an evidence child that attaches a blob
      When a user runs `tektite archive <ref>`
      Then the node file moves to .tektite/archive/nodes/
      And its events move to .tektite/archive/events/<ref>/
      And its evidence child and the blobs only that evidence uses move with it
      And the result lists the archived nodes

    Scenario: A blob that an active node still uses stays in place
      Given a blob attached to evidence under a done ticket and to evidence under an open ticket
      When a user archives the done ticket
      Then the blob stays in .tektite/blobs/

    Scenario: An open node cannot be archived
      Given a ticket that is open, active, blocked or in review
      When a user runs `tektite archive <ref>`
      Then it exits with code 4 and says only done or dropped nodes can be archived
      And nothing moves

    Scenario: A node with open children cannot be archived
      Given a done plan with an open ticket under it
      When a user runs `tektite archive <ref>`
      Then it exits with code 4 and names the open child
      And nothing moves

    Scenario: A node that blocks an open node cannot be archived
      Given a done ticket that blocks an open ticket
      When a user runs `tektite archive <ref>`
      Then it exits with code 4 and names the blocked node
      And nothing moves

    Scenario: Archiving several nodes is all or nothing
      Given a done ticket and an open ticket
      When a user runs `tektite archive` with both references
      Then it exits with code 4 and nothing moves

    Scenario: An archived node is not archived again
      Given an archived node
      When a user runs `tektite archive <ref>`
      Then it exits with code 4 and says the node is already archived

  Rule: Closed nodes can be archived in bulk by age

    Scenario: Archiving by age takes closed nodes whose last event is old enough
      Given done and dropped tickets closed 40 days ago, 10 days ago, and an open ticket
      When a user runs `tektite archive --done --older-than 30d`
      Then only the nodes closed more than 30 days ago move
      And a duration accepts days, weeks and the units of Go durations

    Scenario: Bulk archiving skips nodes that are not safe to move
      Given an old done ticket with an open child and an old done ticket that blocks an open ticket
      When a user runs `tektite archive --done --older-than 30d`
      Then they are skipped with a reason and the command exits with code 0

    Scenario: An archive request must name nodes or select them
      When a user runs `tektite archive` with no reference and no --done
      Or runs it with --older-than but without --done
      Or runs it with both references and --done
      Then it exits with code 2

    Scenario: An unknown duration is a usage error
      When a user runs `tektite archive --done --older-than soon`
      Then it exits with code 2

  Rule: A dry run changes nothing

    Scenario: A dry run lists what would move
      Given a done ticket
      When a user runs `tektite archive <ref> --dry-run`
      Then the result lists the node as it would be archived
      And no file moves

    Scenario: Empty results are empty lists
      Given no node can be archived
      When a user runs `tektite archive --done --older-than 30d --json`
      Then the archived and skipped lists are `[]`, not null

  Rule: Archived nodes stay reachable

    Scenario: Show and references resolve archived nodes
      Given an archived ticket
      When a user runs `tektite show <ref>`
      Then the node is shown with its state and marked as archived
      And a hash suffix resolves to it as before

    Scenario: Archived nodes are left out of ls unless asked
      Given an archived ticket and an active ticket
      When a user runs `tektite ls`
      Then only the active ticket is listed
      And `tektite ls --archived` and `tektite ls --all` list both, with archived nodes marked

    Scenario: Archived nodes are left out of next and context
      Given an active ticket linked to an archived node
      When a user runs `tektite next` and `tektite context <ref>`
      Then the archived node is neither suggested nor part of the bundle
      And `tektite context <ref> --archived` includes it

    Scenario: Check passes with archived nodes and does not break links to them
      Given an active ticket that links to and has a parent that is archived
      When a user runs `tektite check`
      Then no problem is reported
      And `tektite check --archived` also checks the archived nodes

    Scenario: Archived nodes cannot be written to
      Given an archived ticket
      When a user runs `tektite note <ref> "text"`
      Then it exits with code 4 and says to unarchive it first

  Rule: Unarchiving reverses the move

    Scenario: Unarchiving restores the node, its events and blobs
      Given an archived ticket with events and an evidence child with a blob
      When a user runs `tektite unarchive <ref>`
      Then the files return to .tektite/nodes/, .tektite/events/ and .tektite/blobs/
      And the node appears in `tektite ls` again

    Scenario: Unarchiving a node that is not archived is a conflict
      Given an active ticket
      When a user runs `tektite unarchive <ref>`
      Then it exits with code 4

    Scenario: Unarchive supports a dry run
      Given an archived ticket
      When a user runs `tektite unarchive <ref> --dry-run`
      Then the result lists the node and no file moves
