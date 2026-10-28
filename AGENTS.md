# AGENTS.md - Artemis

> Read this file at session start and update it at the end of meaningful sessions.

## Current Direction

- The user requested a fresh start on 2026-10-28.
- The previous implementation, documentation, runtime data, and dependencies have been backed up outside the repository.
- This repository currently contains only project guidance and ignore rules. No application, dependency configuration, or test suite is present.
- The official product name is **Artemis**; the existing repository and local folder names remain unchanged.
- The initial product direction is a multi-agent development harness. This is an early concept, not a detailed implementation specification.
- This chat is explicitly assigned to planning. The user supplies milestones; elaborate each into scope, UI, responsibilities, flows, and acceptance scenarios. Keep the overall plan concise and do not implement the application or invent later milestones. Milestone-specific design is now authorized.
- Codex OAuth authentication, model invocation/response checking, and tool use are the user's explicit milestone-1 goals. The user selected Artemis-owned Sign in with ChatGPT authentication. Artemis owns the OAuth lifecycle; Codex-managed login is not the selected route. The exact model and execution integration remain undecided. No authentication or inference test has been performed for Artemis.
- Obsidian is the intended way to organize macro-level project memory. The current outline distinguishes per-agent working memory, shared results/handoffs, and macro memory. Storage, retrieval, and synchronization are deferred. `docs/memory-architecture.md` is retained only as future research reference; its central service, SQLite, and other detailed proposals are not current planning decisions.
- The agent outline proposes coordination, task execution, and verification roles. The draft flow is request, planning/delegation, execution, integration/verification with correction loops, and reporting/memory updates. Independent work can run in parallel; dependent work runs in order. This is product planning, not an instruction to spawn agents in this chat. Agent counts, specialties, model assignment, communication, and execution controls remain deferred.
- The harness outline covers agent/task execution control, model/tool connections, progress/error tracking, and memory access/recording. Agents make task judgments; the harness supports their execution and coordination. Detailed control and recovery behavior remain deferred.
- The product targets both individuals and teams and aims to handle a broad range of tasks as a general-purpose development harness. The MVP focuses on individual use.
- Product autonomy is explicitly limited: agents must not act beyond the user's instructions, and ongoing user judgment is expected. Do not infer expanded scope or invent a detailed approval protocol; specific intervention points remain to be designed.
- The operating direction is local execution with a Windows desktop GUI using .NET + WPF + C#, explicitly selected by the user. Proposed areas are projects, requests/conversation, task/agent progress, results/memory, and settings. Layout, .NET version, and detailed architecture remain undecided; this planning decision does not authorize building the application yet.
- Retries are limited to 5. The user does not intend to impose task-time or usage limits. Completion and interruption criteria still need separate definition.
- The MVP scope is basic multi-agent functionality plus an Obsidian-integrated memory system. The user defines milestones and this chat elaborates the supplied goals; implementation requires separate instructions.
- Milestone 1 is desktop-UI harness foundation design. `docs/milestone-01-harness.md` contains a planning draft for authentication, single-conversation model requests/responses, tool use, user controls, and future verification scenarios. .NET + WPF + C# and app-owned Sign in with ChatGPT are selected. Codex app-server, the single-conversation baseline, tool choices, and per-tool confirmation remain proposals. Model, execution engine, tool scope, retry counting, and authentication details remain open. Multi-agent/Obsidian integration remains in the overall MVP scope.
- The user requires authentication without asking users to install Codex or another app, because Codex cannot be assumed present and a separate authentication app creates onboarding friction. Use Artemis with system-browser sign-in. Treat runtime packaging separately from authentication: app-server use and bundling versus direct Responses integration are undecided; do not assume a preinstalled Codex dependency or infer that the user has rejected all embedded Codex runtime use. The earlier Tauri/Rust and managed-login recommendation was not adopted. Official app-server authentication restrictions, experimental status, and direct-token route constraints remain research references; no live connection or performance validation has occurred.
- The basic desktop stack and authentication ownership are selected; detailed architecture and remaining technologies are undecided. `README.md` records the current concise planning outline and official connection references.
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
- At reset verification, the project root contained only `.git`, `.env`, `.gitignore`, `AGENTS.md`, and `README.md`. The `docs` directory now contains memory research and milestone-1 planning; there is still no application code or dependency configuration.
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
| #78 | 2026-10-28 | Reviewed the current first-stage plan and prepared a concise consolidated summary plus potential missing planning areas for user review. Did not add product requirements or elaborate the design. Recorded that the user will identify omissions and lead subsequent stepwise planning. Verified repository structure, secret exclusion, and `git diff --check`. |
| #79 | 2026-10-28 | Recorded the user's planning decisions: individual/team scope with a personal MVP, strictly instruction-bounded autonomy with ongoing user judgment, local desktop GUI, 5 retries without task-time/usage caps, and a basic multi-agent plus Obsidian-memory MVP. Left completion/interruption criteria open and milestone definition to the user. No implementation added. Verified repository structure, secret exclusion, and `git diff --check`. |
| #80 | 2026-10-28 | Recorded this chat's planning-only role and elaborated the user-supplied first milestone: desktop harness design for Codex OAuth, model calls/response checking, and tool use. Consulted official OpenAI docs and documented candidate integration, UI areas, controls, flows, open decisions, and future verification scenarios. No application or live connection testing. Verified repository structure, local document links, secret exclusion, and `git diff --check`. |
| #81 | 2026-10-28 | Compared desktop technologies, Codex integration, and authentication using official OpenAI, Tauri, Electron, and Microsoft docs. Recommended Tauri/React/TypeScript with a Rust harness, app-server stdio, and managed browser login for personal milestone-1 validation; retained all choices as unapproved recommendations. Recorded app-server experimental status, official Sign in with ChatGPT migration guidance, commercial/hosted authentication restrictions, and direct-token route constraints. No implementation or live tests. Verified repository structure, local document links, secret exclusion, and `git diff --check`. |
| #82 | 2026-10-28 | Recorded the user's selection of .NET + WPF + C# and Artemis-owned Sign in with ChatGPT. Added the requirement to authenticate without a separate Codex/app installation and a Codex-absent login acceptance scenario. Reconciled README, milestone design, and current guidance; kept execution-engine selection and runtime packaging separate and undecided. No implementation or live tests. Verified repository structure, local document links, secret exclusion, and `git diff --check`. |
