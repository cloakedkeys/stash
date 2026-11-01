# Working in this repository

This is a private copy of Stash. The default branch is `develop`. Agents read this file, and Git is how they share what they've done.

## Who does what

One agent writes and another reviews. The reviewer reads the diff and reports findings. It doesn't edit unless explicitly asked.

- Each piece of work has its own branch and its own worktree. Never edit a worktree another agent is using.
- Keep context where the next agent can find it: commit messages, PR descriptions, and review comments on the PR.
- Every review is posted as a comment on its PR, listing each finding and what was done about it.

## Rules

- **No AI attribution anywhere.** No `Co-Authored-By`, "Generated with" or session links in commits, PR descriptions or comments.
- **One concern per PR.** If you find a bug that's out of scope, report it for a separate issue. Don't fix it in the same PR.
- **Try the simplest fix first.** Don't add machinery, such as caching or new abstractions, when a plain change solves the problem.
- **Fix the cause, with a test that fails on the old code.** Check that it does fail.
- **Report what actually happened.** State what was tested, and what wasn't.
- Files use LF line endings.
- Don't rewrite published history or move tags.

## Writing

Issues, PRs and changelogs use plain language: say what the user sees and what changed. Jargon and code go in a `<details>` block. Keep it short.

## Bugs that came from upstream

Check the discussion on `stashapp/stash` first, and port an upstream fix if there is one. Otherwise, follow standard practice.

## Multiplatform

Stash runs on Windows, Linux, macOS and Android, and on both SQLite and PostgreSQL. A change has to be right on all of them, including ones you can't run.

## Tests

```shell
make generate-backend
go test -count=1 -tags "sqlite_stat4 sqlite_math_functions integration" ./pkg/...
```

Run `./internal/...` as well when a change touches shared code. Its tests use shared helpers and fixtures too.

For the PostgreSQL tests, set `PGSQL_TEST` to the address of a test database and run `./pkg/database` and `./pkg/postgres` one package at a time, using `-p 1`. See `.github/workflows/test-postgres.yml`.

For the UI, run these in `ui/v2.5`. A new test file is found automatically.

```shell
pnpm run gqlgen
pnpm run validate
pnpm run test
```

These checks must pass before a PR can merge: `generate`, `lint`, `test` and `test-postgres`. A docs-only PR skips all but `generate`, and a skipped check counts as passed. Don't weaken, skip or delete a test to get a pass. If a test fails because the code has moved on, say so in the PR.

## Reviewing

Tests passing isn't enough, because a fix can pass every test and still break something next to it.

- **Run the change.** Anything a user can see gets tried on a real server, and in a real browser if it's in the UI. Change what a user would change, and then look at what's next to it.
- **Use throwaway data.** Run each server on its own test library and database, never on someone's real one.
- **Compare with the old build when behaviour changes.** Do the same steps on both, each with its own copy of the database, because an old build can't safely open a database a newer one has upgraded. Put the results in the review. Some of what looks like a new bug is old, and some of what looks fixed was never broken.
- **Check what the fix could break, not only what it fixes.** Saved choices, existing data, old backups, tools and other clients.
- **Say what wasn't tried.** Every review ends with what wasn't checked.

## Bugs

Before fixing a bug, check whether it shares a cause with others. Fix the cause and close them together, rather than patching each report. Check each report against the fix before closing it.
