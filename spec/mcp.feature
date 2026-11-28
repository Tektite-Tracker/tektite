Feature: MCP server
  An agent can use tektite through the Model Context Protocol instead of the CLI. `tektite mcp` serves
  tools over stdio, and each tool calls the same service as the matching CLI verb, so the results
  and the refusals are the same.

  Background:
    Given a repository with a .tektite/ directory

  Rule: The server speaks MCP over stdio

    Scenario: Initialising a session
      When a client sends "initialize"
      Then the server answers with its name, its version and the "tools" capability
      And a "notifications/initialized" message gets no answer

    Scenario: Listing the tools
      When a client sends "tools/list"
      Then every tool has a name, a description and an object input schema
      And the tools cover show, ls, next, context, check, new, set-status, note, link, unlink, claim, heartbeat, release, approve, revise, attach-evidence, rerun, close-gap and policy explain

    Scenario: Unknown methods and malformed input are rejected
      When a client sends a method the server does not know
      Then the answer is a JSON-RPC error with code -32601
      When a client sends a line that is not JSON
      Then the answer is a JSON-RPC error with code -32700
      And the server keeps serving

    Scenario: Malformed initialize params are rejected
      When a client sends "initialize" with params that are not a JSON object
      Then the answer is a JSON-RPC error with code -32602
      And "initialize" without params still succeeds

  Rule: Tools wrap the verbs

    Scenario: Reading a node through a tool
      Given a ticket "auth-retry-3fa"
      When a client calls "tektite_show" with ref "auth-retry-3fa"
      Then the result is the same document as `tektite show auth-retry-3fa --json`

    Scenario: Writing through a tool appends the same event as the CLI
      Given a ticket "auth-retry-3fa"
      When a client calls "tektite_set_status" with status "review" and actor "agent:claude"
      Then a set-status event is appended
      And the result names the event, the actor and the path

    Scenario: A refused write is a tool error with its kind and hint
      Given a policy that requires passing evidence before done
      When a client calls "tektite_set_status" with status "done"
      Then the result is marked as an error
      And it carries the kind "policy", the hint, and the failed gates
      And no event is appended

    Scenario: Warnings are returned as diagnostics
      When a client calls "tektite_note" without an actor and TKT_ACTOR is unset
      Then the result lists a diagnostic with code "TKT-W006"

    Scenario: Evidence can be attached from text
      When a client calls "tektite_attach_evidence" with text and a claim
      Then a blob and an evidence node are created like `tektite attach-evidence --file -`

  Rule: Running commands is opt-in

    Scenario: Commands are not run unless the server allows it
      When a client calls "tektite_attach_evidence" with a run command or calls "tektite_rerun"
      Then the result is an error that says commands are disabled
      And nothing is executed or written
      And `tektite mcp --allow-run` enables both

    Scenario: Archiving through a tool
      Given a done ticket
      When a client calls tektite_archive with its reference
      Then the node is archived exactly as the CLI would
      And tektite_unarchive restores it
