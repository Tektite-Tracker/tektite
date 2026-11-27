# Contributing

Thanks for helping with tektite. Contributions are welcome as pull requests.

## Before you start

Read AGENTS.md for the repo layout, the make targets, the test conventions and the comment policy. Behaviour is defined in the feature files under `spec/`, so change or add the scenario there first when you change behaviour.

## Making a change

- Branch from an up-to-date `main`, named `{type}/{short_desc}` in lowercase, e.g. `feat/node-event-fold`.
- Keep one piece of work per branch and per pull request.
- Run `make lint`, `make test` and `make build` before pushing, and wait for CI to pass.
- Write each commit as a single Conventional Commit line (`feat:`, `fix:`, `chore:`, `docs:`).
- Describe the pull request with a one-line summary and a short paragraph on why the change exists.

## License

By contributing you agree that your work is released under the MIT license in LICENSE.
