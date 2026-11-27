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

## Releases

Every merge to `main` can cut a release, and the commit types decide the version: `feat:` bumps the minor, `fix:` bumps the patch, and a `!` after the type or a `BREAKING CHANGE` footer bumps the major (held below 1.0, where it bumps the minor). `docs:`, `chore:`, `test:`, `ci:` and `refactor:` do not release. Pick the type for what users see, not for the size of the diff.

## License

By contributing you agree that your work is released under the MIT license in LICENSE.
