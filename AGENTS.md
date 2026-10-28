# AGENTS.md - Artemis Project

> Read this file at session start and update it at the end of meaningful sessions.

## Current Direction

- The user requested a fresh start on 2026-10-28.
- The previous implementation, documentation, runtime data, and dependencies have been backed up outside the repository.
- This repository currently contains only project guidance and ignore rules. No application, dependency configuration, or test suite is present.
- New product requirements, architecture, technology stack, and model provider have not been selected.
- Historical MVP plans and provider choices are reference material, not current requirements. Implement new functionality according to the user's next instructions.

## Preserved State

- Git history and the existing `origin` remote are preserved.
- The pre-reset commit is `7a7a227cb74ed3e3a87ab8994b345f5b75f742b8`.
- The full local backup is at `D:\Artemis_Project_Backups\2026-10-28_before-reset`.
- The backup contains `working-tree`, `repository.bundle`, `files.sha256.json`, and `backup-info.json`.
- The removed working files were moved into the backup's `originals` folder; they were not permanently deleted.
- `.env` is preserved locally and ignored by Git. Do not print or commit its contents.

## Reset Verification

- All 10,593 backup files matched their original sizes and SHA256 hashes, and `git bundle verify` passed.
- The project root contains only `.git`, `.env`, `.gitignore`, `AGENTS.md`, and `README.md`.
- The preserved `.env` hash is unchanged, `.env` remains ignored, and `git diff --check` passed.
- Historical application tests were archived with the implementation; there is no current application test suite to run.

## Session Rules

At session start:

1. Read `AGENTS.md` and `README.md`.
2. Check the current branch and dirty worktree state.

At meaningful completion:

1. Update `AGENTS.md` with current status and a session history entry.
2. Run checks relevant to the change. Until application code exists, verify repository structure, secret exclusion, and `git diff --check` instead of running historical test commands.
3. Commit and push to `origin/main` unless the user asks otherwise.

## Session History

| Session | Date | Work |
|---------|------|------|
| #72 | 2026-10-28 | Reset the project for a fresh start at the user's request. Backed up all pre-reset working files and Git history outside the repository, verified the backup, preserved `.git` and `.env`, and replaced the previous implementation with minimal project guidance. Earlier session history is preserved in the backup and Git history. |
