# AGENTS.md - Artemis

> Read this file at session start and update it at the end of meaningful sessions.

## Current Direction

- The user requested a fresh start on 2026-10-28.
- The previous implementation, documentation, runtime data, and dependencies have been backed up outside the repository.
- This repository currently contains only project guidance and ignore rules. No application, dependency configuration, or test suite is present.
- The official product name is **Artemis**; the existing repository and local folder names remain unchanged.
- The initial product direction is a multi-agent development harness. This is an early concept, not a detailed implementation specification.
- The user clarified that the current phase is concise first-stage planning focused on the overall framework. Keep current planning brief and role-oriented; defer technology selection, schemas, protocols, and implementation detail until a later detailed-design request.
- GPT/Codex with OAuth authentication is a connection candidate. Official documentation describes a Sign in with ChatGPT / Codex app-server route; no authentication or inference test has been performed for Artemis. The exact model, integration, and deployment form remain undecided.
- Obsidian is the intended way to organize macro-level project memory. The current outline distinguishes per-agent working memory, shared results/handoffs, and macro memory. Storage, retrieval, and synchronization are deferred. `docs/memory-architecture.md` is retained only as future research reference; its central service, SQLite, and other detailed proposals are not current planning decisions.
- The agent outline proposes coordination, task execution, and verification roles. The draft flow is request, planning/delegation, execution, integration/verification with correction loops, and reporting/memory updates. Independent work can run in parallel; dependent work runs in order. This is product planning, not an instruction to spawn agents in this chat. Agent counts, specialties, model assignment, communication, and execution controls remain deferred.
- The harness outline covers agent/task execution control, model/tool connections, progress/error tracking, and memory access/recording. Agents make task judgments; the harness supports their execution and coordination. Detailed control and recovery behavior remain deferred.
- The user explicitly selected a GUI interface. Proposed areas are projects, requests/conversation, task/agent progress, results/memory, and settings. Desktop versus web delivery, layout, and implementation technology remain undecided; do not treat the GUI requirement as a request to build it yet.
- Detailed architecture, technology stack, and initial implementation scope have not been selected. `README.md` records the current concise planning outline and official connection reference.
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
- At reset verification, the project root contained only `.git`, `.env`, `.gitignore`, `AGENTS.md`, and `README.md`. Memory research has since added `docs/memory-architecture.md`; there is still no application code or dependency configuration.
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
| #73 | 2026-10-28 | Recorded the official name Artemis and the initial multi-agent development harness concept. Documented GPT/Codex OAuth as an untested connection candidate using official OpenAI references, Obsidian for macro memory, and detailed memory design as open research. No application code or dependencies added. Verified repository structure, `.env` exclusion, and `git diff --check`. |
| #74 | 2026-10-28 | Researched detailed multi-agent memory using primary LangGraph, SQLite, and Obsidian documentation. Added a proposed architecture covering scoped task state, evidence-backed memory, retrieval, concurrent updates, invalidation, restart recovery, and Obsidian ownership/synchronization. Kept all technology choices provisional; no application code, runtime memory, or Vault created. Verified repository structure, secret exclusion, document links, and `git diff --check`. |
| #75 | 2026-10-28 | Applied the user's clarification to keep first-stage planning concise and focused on the overall framework. Simplified the README around working, shared, and macro memory; deferred technical choices and marked the detailed research as future reference. Verified repository structure, secret exclusion, local document links, and `git diff --check`. |
| #76 | 2026-10-28 | Added a concise draft of agent roles and multi-agent operation: coordination, task execution, verification, task-dependent parallelism, correction loops, and reporting/memory updates. Deferred agent counts, model assignments, communication, and execution details. No implementation added. Verified repository structure, secret exclusion, and `git diff --check`. |
| #77 | 2026-10-28 | Recorded the user's GUI requirement and outlined harness responsibilities, proposed GUI areas, and the basic user flow. Kept delivery platform, layout, technology, and execution/recovery details deferred. No implementation added. Verified repository structure, secret exclusion, and `git diff --check`. |
