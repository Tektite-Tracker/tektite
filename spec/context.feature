Feature: Context bundles
  `tektite context <node>` produces one prompt-ready bundle for an agent that starts with no
  memory. The bundle is built from the node, its ancestors, gaps, evidence, and links, and it
  is trimmed to a token budget by priority tiers.

  Background:
    Given a repository with a .tektite/ directory
    And a spec "auth-spec-1aa", a plan "retry-plan-9de" under it, and a ticket "auth-retry-3fa" under the plan

  Rule: The bundle is assembled in priority order

    Scenario: The node is always included
      When an agent runs `tektite context auth-retry-3fa --budget 200`
      Then the output contains the full body of "auth-retry-3fa"

    Scenario: Tiers are filled in order
      Given a budget large enough for everything
      When an agent runs `tektite context auth-retry-3fa`
      Then the output has these sections in order
        | section                   |
        | node                      |
        | ancestor specs and plans  |
        | open gaps                 |
        | recent evidence           |
        | linked nodes              |
        | recent history            |

    Scenario: The current plan revision is used
      Given the plan has revisions 1 and 3
      When an agent runs `tektite context auth-retry-3fa`
      Then the plan section shows revision 3
      And names the revision number

  Rule: The budget is respected

    Scenario: Bodies shrink to summaries when over budget
      Given a budget too small for all ancestor bodies
      When an agent runs `tektite context auth-retry-3fa --budget 1500`
      Then lower tiers are shown as title and summary only
      And the output states which nodes were shortened

    Scenario: Lower tiers are dropped before higher ones
      Given a very small budget
      When an agent runs `tektite context auth-retry-3fa --budget 300`
      Then linked nodes and history are dropped first
      And the node itself is kept

    Scenario: The bundle reports what it left out
      Given a budget that forces omissions
      When an agent runs `tektite context auth-retry-3fa --budget 800`
      Then the output ends with a list of omitted nodes and how to fetch each

    Scenario: A node that alone exceeds the budget is truncated with a marker
      Given the node body is larger than the budget
      When an agent runs `tektite context auth-retry-3fa --budget 100`
      Then the body is cut at a paragraph boundary
      And a marker says the body was truncated

    Scenario: The default budget comes from configuration
      Given "context_budget: 8000" in .tektite/config
      When an agent runs `tektite context auth-retry-3fa` without a flag
      Then the budget is 8000

  Rule: Evidence and gaps are shown with what matters

    Scenario: Only open gaps are listed
      Given a node with two open gaps and one closed gap
      When an agent runs `tektite context`
      Then the two open gaps are listed with their block ids
      And the closed gap is not listed

    Scenario: Evidence shows claim and provenance but not binary content
      Given an evidence node with a screenshot attachment
      When an agent runs `tektite context`
      Then the evidence is shown with its claim, commit, and the blob path
      And the image bytes are not included

    Scenario: Stale evidence is flagged
      Given evidence recorded at an older commit than the current HEAD
      When an agent runs `tektite context`
      Then the evidence is marked as possibly stale

  Rule: Output formats serve agents and tools

    Scenario: JSON output
      When an agent runs `tektite context auth-retry-3fa --json`
      Then the output is a JSON object with the sections as keys
      And each entry carries its node reference and token estimate

    Scenario: Markdown output is the default
      When a user runs `tektite context auth-retry-3fa`
      Then the output is markdown with one heading per section

    Scenario: Token counts are estimates
      When the bundle is built
      Then sizes are estimated as the byte length divided by 4, rounded up
      And the budget is a soft limit that only the node itself can exceed, by its truncation marker
      And without a flag or a configured value the budget is 8000

    Scenario: Unknown nodes fail clearly
      When an agent runs `tektite context ghost-000`
      Then the command exits with code 3
