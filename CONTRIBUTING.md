# Contributing

## Before Coding

1. Read `AGENTS.md`, `NOTICE`, `PATCHES.md`, and `docs/yanchuaner/copyright-matrix.md`.
2. State whether the change is upstream code, an authorized modification, original Yanchuaner work, or a replacement design.
3. For billing, identity, keys, BYOK, or migrations, add a design note and rollback plan before implementation.
4. Keep production secrets and personal data out of issues, commits, fixtures, logs, screenshots, and PRs.

## Originality and Licensing

- Do not mark renamed, reformatted, translated, or AI-rewritten upstream code as original.
- Preserve New API and all third-party copyright headers, licenses, notices, and UI attribution.
- New original files should identify their actual author or contributor group and the license under which they are contributed.
- The project has not adopted a copyright assignment agreement. A contribution does not transfer copyright to an unspecified operating entity.
- AI-assisted changes must be reviewed by a human contributor who understands the behavior, license origin, tests, and security impact.

## Verification

Run the smallest relevant tests first, then the affected package suite, frontend typecheck/lint/build, Compose parsing, migration checks on SQLite and PostgreSQL, and secret scanning. PR descriptions must record commands, results, known failures, data migration, and rollback.

Stage only the intended files. Do not include `.env`, databases, backups, local toolchains, caches, or unrelated worktree changes.
