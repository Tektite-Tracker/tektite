Feature: Embedded documentation
  The markdown files under docs/ are the single source. They render on GitHub and are embedded
  in the binary, so `tektite docs` works offline and always matches the installed version.

  Rule: Topics are the embedded markdown pages

    Scenario: Listing the topics
      When a user runs `tektite docs`
      Then each embedded page is listed with its topic name and title
      And the index topic is listed first

    Scenario: Listing the topics as JSON
      When a user runs `tektite docs --json`
      Then a JSON array of objects with a topic and a title is printed

    Scenario: Printing a topic
      When a user runs `tektite docs design`
      Then the markdown of docs/design.md is printed unchanged

    Scenario: A nested topic is addressed by its path
      Given a page under a docs/ subdirectory
      When a user runs `tektite docs` with that path without the .md suffix
      Then the page is printed

    Scenario: An unknown topic is refused
      When a user runs `tektite docs nope`
      Then the command exits with code 3
      And the error says to run `tektite docs` to list the topics

    Scenario: The index lists every topic
      Given the embedded pages
      Then docs/index.md links to every page other than itself

    Scenario: Docs need no repository
      When a user runs `tektite docs` outside a directory that holds .tektite/
      Then the topics are listed
