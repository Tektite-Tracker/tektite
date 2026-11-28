Feature: Store
  The store reads .tektite/ from disk into a snapshot of nodes, events, and the tree, and
  writes new node files. Reading never fails on bad content, and writing never replaces a file.

  Background:
    Given a repository with a .tektite/ directory

  Rule: Nodes and events are loaded leniently

    Scenario: Nodes and events are loaded from disk
      Given nodes in .tektite/nodes/ and events in .tektite/events/<ref>/
      When the repository is loaded
      Then every node is listed in order of reference
      And each node carries its events and the diagnostics of its files

    Scenario: Only visible markdown files are read
      Given a hidden file, a temporary file, and a non-markdown file in .tektite/nodes/
      When the repository is loaded
      Then they are ignored

    Scenario: A repository without nodes or events is empty
      Given a .tektite/ directory with no nodes or events directories
      When the repository is loaded
      Then no node is listed and no error is reported

    Scenario: A parent reference resolves by hash suffix
      Given a ticket whose parent is "old-plan-name-9de" and a plan named "retry-plan-9de"
      When the tree is built
      Then the plan is the ticket's parent

    Scenario: An unresolved parent is kept and the node becomes a root
      Given a ticket whose parent matches no node
      When the tree is built
      Then the ticket is listed as a root
      And the resolution error is available to `tektite check`

    Scenario: Events resolve to their node by hash when the directory slug is stale
      Given events under .tektite/events/old-slug-3fa/ and a node "auth-retry-3fa"
      When the repository is loaded
      Then the events belong to "auth-retry-3fa"

    Scenario: Descendants are listed in tree order
      Given a spec, a plan under it, and two tickets under the plan
      When the descendants of the spec are listed
      Then the plan comes first, then its tickets by reference, with their depth

  Rule: New node files are written atomically

    Scenario: A new node file is written atomically
      When a node file is written for "add-retry-3fa"
      Then .tektite/nodes/add-retry-3fa.md exists with the given content
      And no temporary file is left behind

    Scenario: An existing node file is never overwritten
      Given the file .tektite/nodes/add-retry-3fa.md exists
      When a node file is written for "add-retry-3fa"
      Then the write fails with a conflict
      And the existing file is unchanged

    Scenario: An unsafe reference is rejected
      When a node file is written for a reference containing a path separator or a leading dot
      Then the write fails and nothing is created
