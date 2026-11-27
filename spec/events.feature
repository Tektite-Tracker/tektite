Feature: Events
  An event is a small markdown file in .tektite/events/<node-ref>/<ulid>.md. Events are
  append-only and carry all volatile state: status, claims, comments, links added later,
  plan revisions, and evidence attachments. A node's current state is a fold over its events.
  Writers add files and never edit shared ones, so concurrent work merges cleanly in git.

  Background:
    Given a repository with a .tektite/ directory
    And a ticket "auth-retry-3fa"

  Rule: The built-in event types are a closed, documented list

    Scenario Outline: Each type has defined fields and effect
      Given an event of type "<type>" with its required fields "<required>"
      When the node's state is computed
      Then the effect is "<effect>"

      Examples:
        | type       | required                  | optional            | effect                                          |
        | set-status | status                    | override            | sets the node's status                          |
        | claim      | ttl                       |                     | starts a lease held by the actor                |
        | heartbeat  |                           |                     | extends the actor's live lease                  |
        | release    |                           |                     | ends the actor's lease                          |
        | note       |                           |                     | adds a comment to the history                   |
        | link       | rel, target               |                     | adds a link                                     |
        | unlink     | rel, target               |                     | removes a link                                  |
        | revision   | number                    |                     | sets a new plan body from the event body        |
        | attach     | evidence                  | block               | attaches an evidence node to the node or block  |
        | close      | block                     | evidence            | marks a gap or checklist block as closed        |
        | approve    | revision                  |                     | approves a plan revision                        |

    Scenario: An override is a field on set-status, not a separate type
      Given a set-status event with the field "override" set to "accepted risk"
      When the node's state is computed
      Then the status changes as usual
      And the history marks the event as an override with that reason

    Scenario: A missing required field makes the event invalid
      Given a claim event with no "ttl"
      When `tkt check` runs
      Then an error names the event and the missing field
      And the event is skipped in the fold

    Scenario: The list can only grow in a minor version
      Given a newer event type unknown to this version
      When the node's state is computed
      Then the event is treated as an unknown type and kept in the history

  Rule: An event file is markdown with frontmatter

    Scenario: Parsing an event
      Given the file .tektite/events/auth-retry-3fa/01HZ3M0A1B2C3D4E5F6G7H8J9K.md contains
        """
        ---
        type: set-status
        actor: agent:claude-opus/s-4f2a
        at: 2026-11-27T10:15:00Z
        status: active
        ---
        Starting on the retry logic.
        """
      When the event is parsed
      Then its type is "set-status"
      And its actor is "agent:claude-opus/s-4f2a"
      And its time is 2026-11-27T10:15:00Z
      And its field "status" is "active"
      And its body is "Starting on the retry logic."

    Scenario: An event file whose name is not a ULID is a warning
      Given the file .tektite/events/auth-retry-3fa/first-try.md is a valid event
      When the event is parsed
      Then a warning says the file name is not a ULID
      And the event stays valid
      And in the fold it is ordered by "at", then by file name

    Scenario: The event time is a YAML timestamp or an RFC 3339 string
      Given an event whose "at" is an unquoted timestamp, and one whose "at" is the text "yesterday"
      When the events are parsed
      Then the first has that time
      And the second is invalid with a time error

    Scenario: An empty required field counts as missing
      Given a link event whose "target" is empty
      When the event is parsed
      Then the event is invalid and the error names "target"

    Scenario: Events are ordered by time, then ULID
      Given two events where the first has an earlier "at", and two events with equal "at" and different ULIDs
      When the events are folded
      Then they are applied in order of "at"
      And events with equal "at" are applied in ULID order

    Scenario: A skewed clock does not change the order
      Given an event with the greater ULID whose "at" is earlier than another event's "at"
      When the events are folded
      Then they are still applied in order of "at", then ULID
      And no warning is reported for the skew

    Scenario: The actor is declared and unsigned
      Given an event with actor "human:jesus"
      When the event is parsed
      Then the actor is recorded as written
      And no signature is verified

    Scenario: A missing actor is repaired from the environment
      Given an event with no actor
      When `tkt check` runs
      Then a warning says the actor is missing
      And the parser leaves the actor empty and the event stays valid
      And callers repair the actor from the environment, else attribute it to "unknown"

    Scenario: Unknown event types are kept and ignored by the fold
      Given an event of type "reaction"
      When the node's state is computed
      Then the event is listed in the node's history
      And it does not change status, claims, or links

  Rule: Status is the latest set-status event

    Scenario: A node with no events is open
      Given a ticket with no events
      When the node's state is computed
      Then its status is "open"

    Scenario: The latest status event wins
      Given events "set-status: active" then "set-status: review" then "set-status: done"
      When the node's state is computed
      Then its status is "done"

    Scenario: Concurrent status changes resolve by time then ULID
      Given two set-status events from different actors with the same timestamp
      When the node's state is computed
      Then the event with the greater ULID wins
      And `tkt check` reports the concurrent change as a note

    Scenario: The shared lifecycle states are known
      Given a set-status event with status "blocked"
      When the event is validated
      Then the status is one of open, active, blocked, review, done, dropped

    Scenario: An unknown status is accepted with a warning
      Given a set-status event with status "parked"
      When `tkt check` runs
      Then a warning names the event and the unknown status
      And the computed status is "parked"

    Scenario: A status is trimmed but never case-folded
      Given a set-status event with status "  done  "
      And a set-status event with status "Done"
      When the node's state is computed
      Then the first event's status is "done" with no warning
      And the second event's status stays "Done" with a warning that it differs in case from "done"

    Scenario: A kind schema can restrict statuses
      Given a schema for kind "gap" allowing only open and done
      And a set-status event with status "review" on a gap
      When `tkt check` runs
      Then a warning names the event and the disallowed status

  Rule: Claims are expiring leases

    Scenario: Claiming an open node
      Given the ticket is unclaimed
      When agent "agent:claude-opus/s-4f2a" runs `tkt claim auth-retry-3fa`
      Then a claim event is appended with a ttl of 30 minutes
      And the node's holder is "agent:claude-opus/s-4f2a"
      And the command exits with code 0

    Scenario: A claim blocks other actors while the lease is live
      Given the ticket is claimed by "agent:a" and the lease has not expired
      When "agent:b" runs `tkt claim auth-retry-3fa`
      Then no event is appended
      And the command exits with code 4
      And the output names the holder and the lease expiry

    Scenario: A heartbeat extends the lease
      Given the ticket is claimed by "agent:a" with 5 minutes left
      When "agent:a" runs `tkt heartbeat auth-retry-3fa`
      Then a heartbeat event is appended
      And the lease now expires 30 minutes after the heartbeat

    Scenario: An expired lease can be taken over
      Given the ticket was claimed by "agent:a" and the lease expired 10 minutes ago
      When "agent:b" runs `tkt claim auth-retry-3fa`
      Then a claim event is appended for "agent:b"
      And the holder is "agent:b"

    Scenario: Only the holder can heartbeat or release
      Given the ticket is claimed by "agent:a"
      When "agent:b" runs `tkt release auth-retry-3fa`
      Then no event is appended
      And the command exits with code 4

    Scenario: Releasing frees the node immediately
      Given the ticket is claimed by "agent:a"
      When "agent:a" runs `tkt release auth-retry-3fa`
      Then a release event is appended
      And the node has no holder

    Scenario: Simultaneous claims are resolved deterministically
      Given two claim events on the same node from different actors that merged from separate branches
      When the node's state is computed
      Then the claim with the lower ULID holds the lease
      And `tkt check` reports the losing claim as a conflict note

    Scenario: The lease time comes from the event, not the clock of the reader
      Given a claim event at 10:00 with a 30 minute ttl
      When the state is computed at 10:20
      Then the lease is live
      And when the state is computed at 10:31 the lease is expired

  Rule: Comments, notes, and links are events

    Scenario: Adding a note
      When an actor runs `tkt note auth-retry-3fa "backoff needs jitter"`
      Then a note event is appended with that body
      And the node's history shows the note

    Scenario: A late link is added by event
      When an actor runs `tkt link auth-retry-3fa implements retry-plan-9de`
      Then a link event is appended with type "implements" and target "retry-plan-9de"
      And the folded node has that link in addition to those in its frontmatter

    Scenario: Removing a late link is a new event
      Given a link event to "retry-plan-9de"
      When an actor runs `tkt unlink auth-retry-3fa implements retry-plan-9de`
      Then an unlink event is appended
      And the folded node no longer has that link
      And the original link event is still in the history

  Rule: Plan revisions are snapshot events

    Scenario: Revising a plan
      Given a plan node "retry-plan-9de"
      When an actor runs `tkt revise retry-plan-9de` with a new body
      Then a revision event is appended containing the full new body
      And the revision gets the next number for that plan

    Scenario: The current plan body is the latest revision
      Given a plan with revisions 1, 2, and 3
      When the plan is shown
      Then the body shown is revision 3

    Scenario: Diffing two revisions
      Given a plan with revisions 1 and 3
      When a user runs `tkt diff retry-plan-9de 1 3`
      Then the output is a unified diff of the two snapshot bodies

    Scenario: Concurrent revisions both survive
      Given two revision events with the same number from different branches
      When the plan's state is computed
      Then both revisions are kept in the history ordered by ULID
      And `tkt check` reports that the plan has diverged

    Scenario: Work records which revision it was built against
      Given a ticket implementing the plan at revision 2
      And the plan is now at revision 4
      When `tkt check` runs
      Then a note says the ticket was built against an older revision

  Rule: Evidence is attached by event with provenance

    Scenario: Attaching evidence to a node
      When an actor runs `tkt attach-evidence auth-retry-3fa --file shot.png --claim "retry banner shows"`
      Then the file is stored under .tektite/blobs/ by content hash
      And an evidence node is created with the parent "auth-retry-3fa"
      And an attach event on the ticket references the new evidence node

    Scenario: Provenance is captured with the evidence
      Given evidence produced by running "go test ./..." with exit code 0
      When the evidence node is created
      Then it records actor, time, commit SHA, command, exit code, and claim

    Scenario: Evidence without a commit SHA is flagged
      Given evidence created outside a git repository state
      When `tkt check` runs
      Then a warning says the evidence cannot be tied to a code version

  Rule: Event application is tolerant

    Scenario: An event for a missing node is kept and reported
      Given an event under .tektite/events/ghost-000/
      When `tkt check` runs
      Then a warning names the orphan event
      And other nodes are unaffected

    Scenario: Duplicate event ids are a warning and both events are kept
      Given two events of one node that share the same id
      When the node's state is computed
      Then a warning names the duplicated id
      And both events are listed in the node's history

    Scenario: A malformed event is skipped in the fold
      Given an event whose frontmatter is invalid
      When the node's state is computed
      Then the event is skipped
      And `tkt check` reports the file and the problem

    Scenario: Folding is deterministic
      Given the same set of event files on two machines
      When state is computed on each
      Then the results are identical regardless of file system order
