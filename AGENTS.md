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

For the PostgreSQL tests, set `PGSQL_TEST` to the address of a test database and run `./pkg/database` and `./pkg/postgres` one package at a time, using `-p 1`. See `.github/workflows/test-postgres.yml`.
