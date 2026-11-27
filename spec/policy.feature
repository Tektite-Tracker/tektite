Feature: Policy gates
  A versioned policy file in .tektite/ states rules about who may do what and when. Gates are
  enforced by verbs and reported by `tkt check`, and they are visible in the UI. They are rules
  for the team's process; they are not security, since anyone with write access to the repo
  can edit the files.

  Background:
    Given a repository with a .tektite/ directory

  Rule: Policy is a versioned file

    Scenario: Loading the policy
      Given the file .tektite/policy.md defines gates in frontmatter
      When any command runs
      Then the policy is read from that file

    Scenario: No policy means no gates
      Given no policy file exists
      When any verb runs
      Then no gate is applied

    Scenario: A malformed policy is reported, not enforced half-way
      Given a policy file with invalid frontmatter
      When `tkt check` runs
      Then an error names the problem
      And verbs run with no gates and print a warning

  Rule: Approval gates

    Scenario: A plan needs human approval before agents claim its tickets
      Given a policy "plans require human approval before work"
      And a plan with no approval event
      When an agent runs `tkt claim` on a ticket under that plan
      Then no event is appended
      And the command exits with code 5
      And the output says the plan is not approved

    Scenario: A human approval opens the gate
      Given a plan with an approve event from "human:jesus"
      When an agent runs `tkt claim` on a ticket under that plan
      Then the claim succeeds

    Scenario: An approval by an agent does not count
      Given a plan with an approve event from "agent:claude-opus/s-4f2a"
      When the gate is checked
      Then the plan is still not approved

    Scenario: A new revision invalidates the approval
      Given a plan approved at revision 2
      And a new revision 3
      When the gate is checked
      Then the plan is not approved
      And a note says approval was for revision 2

  Rule: Completion gates

    Scenario: A ticket needs passing evidence before done
      Given a policy "tickets require passing evidence before done"
      And a ticket with no evidence
      When an actor runs `tkt set-status auth-retry-3fa done`
      Then no event is appended
      And the command exits with code 5

    Scenario: Open gaps block completion
      Given a policy "no open gaps before done"
      And a ticket with one open gap
      When an actor runs `tkt set-status auth-retry-3fa done`
      Then the command exits with code 5
      And the output lists the open gap

    Scenario: A human can override a gate with a reason
      Given a failing completion gate
      When "human:jesus" runs `tkt set-status auth-retry-3fa done --override "accepted risk"`
      Then the status event is appended with the override reason
      And the history marks the override

    Scenario: An agent cannot override
      Given a failing completion gate
      When an agent runs `tkt set-status auth-retry-3fa done --override "skip"`
      Then no event is appended
      And the command exits with code 5

  Rule: Gates are scoped by kind and label

    Scenario: A gate applies only to listed kinds
      Given a gate scoped to kind "ticket"
      When the status of a "gap" is set to "done"
      Then the gate does not apply

    Scenario: A gate can be scoped by label
      Given a gate scoped to label "security"
      And a ticket labelled "security"
      When the gate is checked
      Then the gate applies

  Rule: Gates are visible

    Scenario: check reports violations in existing data
      Given a ticket that reached "done" without evidence while a gate requires it
      When `tkt check` runs
      Then an error names the node, the event, and the gate

    Scenario: A gate can be explained
      When a user runs `tkt policy explain auth-retry-3fa`
      Then the output lists each gate that applies to the node and whether it is met
