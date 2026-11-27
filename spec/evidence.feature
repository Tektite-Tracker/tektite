Feature: Evidence
  Evidence is proof that a claim is true: a screenshot, a log, a test run, a trace. It is a
  node of kind "evidence" whose body explains it, with attachments stored as content-addressed
  blobs and provenance recorded so the proof can be tied to a code state and re-run.

  Background:
    Given a repository with a .tektite/ directory
    And a ticket "auth-retry-3fa"

  Rule: Evidence is a node with a claim

    Scenario: An evidence node
      Given the file .tektite/nodes/retry-banner-shot-5de.md contains
        """
        ---
        id: 01HZ3N5R8T2V4X6Z9B1D3F5H7J
        kind: evidence
        title: Retry banner appears on 429
        parent: auth-retry-3fa
        evidences: [auth-retry-3fa]
        claim: The retry banner is shown after a 429 response
        attachments:
          - blobs/9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08.png
        provenance:
          actor: agent:claude-opus/s-4f2a
          at: 2026-11-27T10:40:00Z
          commit: 3f2a9c1
          command: go test ./client -run TestRetryBanner
          exit_code: 0
        ---
        Screenshot taken after the mock server returned 429.
        """
      When the node is parsed
      Then its kind is "evidence"
      And its claim is "The retry banner is shown after a 429 response"
      And it has one attachment and the provenance fields

    Scenario: Evidence attaches to a node through its parent and an evidences link
      Given an evidence node with parent "auth-retry-3fa"
      When the graph is built
      Then "auth-retry-3fa" lists the evidence under "evidenced-by"

    Scenario: Evidence can target a single block
      Given an evidence node with "evidences: [auth-retry-3fa#01HZ3KB2]"
      When the graph is built
      Then the block "01HZ3KB2" shows the evidence

  Rule: Attachments are content-addressed blobs

    Scenario: Storing an attachment
      When an actor runs `tkt attach-evidence auth-retry-3fa --file shot.png --claim "retry banner shows"`
      Then the file is copied to .tektite/blobs/<sha256>.png
      And the evidence node references that path

    Scenario: Identical files are stored once
      Given two evidence nodes created from the same file
      When the blobs are written
      Then a single blob file exists
      And both nodes reference it

    Scenario: A blob never changes
      Given a stored blob
      When any command runs
      Then the blob file is never rewritten or deleted by tektite

    Scenario: A tampered blob is detected
      Given a blob whose content no longer matches its hash name
      When `tkt check` runs
      Then an error names the blob and the evidence nodes that reference it

    Scenario: A missing blob is reported
      Given an evidence node referencing a blob that does not exist
      When `tkt check` runs
      Then a warning names the node and the missing path
      And the node can still be read

    Scenario: Large files are warned about
      Given an attachment of 12 MB and a size cap of 5 MB
      And the cap is the "blob_cap_mb" value in .tektite/config, 5 when unset
      When a user runs `tkt attach-evidence` with that file
      Then the command prints a warning about repository growth
      And the command exits with code 2 unless `--allow-large` is passed

    Scenario: Unused blobs are listed, not deleted
      Given a blob that no node references
      When a user runs `tkt check`
      Then the blob is listed as unreferenced
      And only `tkt gc --blobs` may remove it

  Rule: Text evidence can be inline

    Scenario: A command output is stored as an inline fenced block
      When an actor runs `tkt attach-evidence auth-retry-3fa --run "go test ./..."`
      Then the command is executed
      And its output is stored in the evidence body inside a fenced block
      And its exit code is recorded in the provenance

    Scenario: Long output is stored as a blob and summarised
      Given command output larger than the inline limit of 4096 bytes
      When the evidence is created
      Then the full output is stored as a blob
      And the body holds the last 20 lines and the path of the blob

    Scenario: Run cannot be combined with a file or a command
      When an actor runs `tkt attach-evidence auth-retry-3fa --run "go test ./..." --file out.txt`
      Then the command exits with code 2
      And no evidence node is created

    Scenario: Secrets are not captured from the environment
      When evidence is created from a command
      Then environment variables are not recorded in the provenance

  Rule: Provenance ties proof to a code state

    Scenario: The commit SHA is captured from the repository
      Given the working tree is at commit 3f2a9c1
      When evidence is created
      Then provenance records commit "3f2a9c1"

    Scenario: A dirty tree is recorded
      Given the working tree has uncommitted changes
      When evidence is created
      Then provenance records "dirty: true"

    Scenario: Evidence outside a repository has no commit
      Given no git repository state is available
      When `tkt check` runs
      Then a warning says the evidence cannot be tied to a code version

    Scenario: Evidence can be checked for staleness
      Given evidence recorded at commit 3f2a9c1
      And files related to the claim changed in later commits
      When a user runs `tkt check --stale`
      Then the evidence is reported as possibly stale with the commits that came after

    Scenario: Evidence can be re-run
      Given evidence with a recorded command
      When a user runs `tkt rerun retry-banner-shot-5de`
      Then the command is executed again
      And a new evidence node is created that supersedes the old one
      And the new node has the same claim and parent as the old one

    Scenario: Evidence without a command cannot be re-run
      Given evidence with no recorded command
      When a user runs `tkt rerun` on it
      Then the command exits with code 2
      And no evidence node is created

  Rule: Evidence closes gaps and gates completion

    Scenario: Evidence can close a gap block
      Given a gap block "01HZ3KB2" and an evidence node targeting it
      When an actor runs `tkt close-gap auth-retry-3fa#01HZ3KB2 --evidence retry-banner-shot-5de`
      Then a close event is appended referencing the evidence
      And the block is shown as closed

    Scenario: Failing evidence cannot close a gap block
      Given a gap block "01HZ3KB2" and an evidence node with exit code 1
      When an actor runs `tkt close-gap` with that evidence
      Then the command exits with code 2
      And no event is appended

    Scenario: Only an open gap block of the node can be closed
      Given a node with an already closed gap block and a checklist item that is not a gap
      When an actor runs `tkt close-gap` on either block or on an unknown block id
      Then the command exits with code 2
      And no event is appended

    Scenario: A policy can require evidence before done
      Given a policy requiring evidence on tickets before status "done"
      And a ticket with no evidence
      When an actor sets its status to "done"
      Then no event is appended
      And the command exits with code 5
      And the output names the policy

    Scenario: Failing evidence does not satisfy the policy
      Given an evidence node with exit code 1
      When the policy for "done" is checked
      Then the evidence does not count
