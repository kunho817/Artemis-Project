HANDOFF CONTEXT
===============

USER REQUESTS (AS-IS)
---------------------
1. (Sessions #1-#4) Build Artemis Project — TUI-based multi-agent coding system in Go
2. "적층식으로 계속해서 쌓아나가는 방식으로 작업이 진행될 것 같아"
3. "기능 구현 전에 자료 조사 및 나와의 토론을 통해 어떻게 만들건지 확립하고 작업 진행하자."
4. "실제 테스트는 내가 직접 해볼게. 테스트 코드 부분은 검증 이후 테스트 자료는 제거하는 식으로 repo를 깔끔하게 유지하도록 하자."
5. "이제 우리 시스템과 유사한 다른 시스템들을 대량으로 조사해보자." (Session #35)
6. "구현난이도가 낮은 것부터 적용해보자." (시그니처 기능 5개)
7. "시맨틱을 먼저 진행하고, 그 다음에 Flow Awareness로 진행하자."

GOAL
----
All planned features (Phase A-F + signature features) are complete. Next steps are open-ended: further dogfooding, new feature directions, or deployment/packaging as the user decides.

WORK COMPLETED
--------------
This was session #35 — the largest single session. I implemented:

- Phase F: UI Enhancement — progress bars in Activity panel, per-agent token tracking, test results panel (4th section), file diff viewer overlay (Ctrl+D), all in tui/
- CLI/Headless Mode — cmd/artemis/headless.go (new, 392 lines). Commands: "artemis chat", "--headless", "--multi", "--race". One-shot and interactive stdin/stdout modes.
- Dogfooding — Built Antimatter Dimensions (React+TS) using Artemis CLI. 12 files generated, npm build PASS. Discovered 8 bugs, all fixed.
- Stabilization — Enter=Send (was Ctrl+Enter, broken on Windows), HTTP Client 180s timeout for all 5 LLM providers (was 0=infinite), Orchestrator timeout 90s->5min, maxToolIter default 20 (was unlimited), per-step timeout 3min, force-kill LSP/MCP child processes, exec.CommandContext for all subprocess calls, Config View expanded to 9 sub-tabs (added Skills+MCP).
- Documentation — README.md (English), docs/getting-started.md, docs/architecture.md, docs/configuration.md (all Korean)
- Performance — ToolDescriptions cached (invalidated on Register), SQLite pragmas (64MB cache, mmap 256MB, busy_timeout 5s, synchronous=NORMAL), async fact usage increment
- Competitive Analysis — Researched 15 systems (Cursor, Windsurf, Claude Code, Aider, Devin, Kilo Code, Augment Code, Cline, OpenCode, Codex CLI, Gemini CLI, claude-flow, Claude Swarm, CrewAI, LangGraph). Identified 5 signature features to adopt.
- Signature Feature 1: ARTEMIS.md auto-loading — searches ARTEMIS.md/AGENTS.md/.artemis/RULES.md, injects into agent prompt at P1 priority (8K tokens). In agent/agent.go LoadProjectRules() + BuildPromptWithContext.
- Signature Feature 2: Hooks system — HookFunc type (pre/post tool execution), 3 built-in hooks: DangerousCommandHook (blocks rm -rf, DROP TABLE etc), FilePathHook (blocks ../), LoggingPostHook. In tools/tools.go.
- Signature Feature 3: Parallel Worktree + Race — tools/worktree.go ParallelWorktreeManager (Create/GetDiff/MergeBack/CleanupAll). CLI --race mode runs same task on 2 providers in isolated worktrees, picks best result. In cmd/artemis/headless.go runRace().
- Signature Feature 4: Semantic Context Engine — memory/codeindex.go CodeIndex (file walker, function/type boundary chunking, 8 language support). VectorStore codeChunks collection. BuildPromptWithContext auto-retrieves top-5 relevant code chunks at P3 priority.
- Signature Feature 5: Flow Awareness — tools/flowtracker.go FlowTracker (file access frequency, recent edits, weighted scoring). FlowAwareHook auto-records in PostHook. BuildPromptWithContext injects "Recent Activity" at P1.

CURRENT STATE
-------------
- go build/vet/test: ALL CLEAN (agent, llm, memory, tools pass)
- Working tree: clean, all pushed to origin/main
- Latest commit: a7ab33d "chore: update AGENTS.md — Flow Awareness added to session #35"
- Binary: ~33MB artemis.exe (not committed, .gitignore'd)
- Total codebase: ~29,000 lines Go + 3,335 lines Python (training/)
- 14 internal packages, 22+ tools, 13 agent roles, 6 overlays

PENDING TASKS
-------------
- No incomplete todos
- Potential future directions discussed with user:
  - Deployment/packaging (goreleaser, GitHub Release)
  - Premium tier testing (Claude/GPT API keys)
  - Multi-language LSP activation (Python pyright, TS)
  - Training pipeline execution (RunPod, deferred)
  - More dogfooding on larger projects

KEY FILES
---------
- AGENTS.md - Complete project knowledge base (86KB, 733 lines) — READ THIS FIRST
- cmd/artemis/main.go - Entry point (TUI + CLI router)
- cmd/artemis/headless.go - CLI/headless runtime (chat, --multi, --race)
- internal/agent/agent.go - BaseAgent + BuildPromptWithContext + LoadProjectRules + FlowContext
- internal/tools/tools.go - ToolExecutor + Hooks + FlowTracker wiring
- internal/tools/flowtracker.go - Flow Awareness (file access tracking)
- internal/memory/codeindex.go - Semantic Context Engine (code chunking + embedding)
- internal/tui/pipeline.go - Orchestrator routing + agent building
- internal/tui/memory_init.go - All subsystem initialization (memory, LSP, MCP, skills, code index)
- internal/agent/roles/prompts.go - All system prompts + BuildOrchestratorPrompt

IMPORTANT DECISIONS
-------------------
- Enter=Send, Shift+Enter=newline (Windows terminals can't handle Ctrl+Enter)
- HTTP Client 180s timeout on all 5 providers (was 0/infinite, caused "context deadline exceeded")
- maxToolIter=20 default (was unlimited, caused runaway loops in dogfooding)
- Task sizing: 1 task = 1 file (multi-file tasks timeout/fail)
- Per-step timeout 3min in Engine (prevents one slow step from consuming entire pipeline)
- Force-kill child processes in LSP/MCP Shutdown() (prevents orphaned gopls)
- ARTEMIS.md loaded at P1 priority (high), Flow context also at P1, Code context at P3
- ToolDescriptions cached and invalidated on Register (was rebuilt every LLM call)
- SQLite: synchronous=NORMAL + 64MB cache + 256MB mmap + 5s busy_timeout

EXPLICIT CONSTRAINTS
--------------------
- "적층식 프로젝트" — incremental, features added/modified frequently
- "TUI도 뭔가 더 추가되거나 수정될 가능성이 있다는 것 기억해주고!"
- "역할 매핑은 웹 서칭을 통해서 적절한 LLM을 매핑해주면 되는데..적용하기 전에 나한테서 확인을 한번 거쳐줘"
- "실제 테스트는 내가 직접 해볼게."
- "테스트 코드 부분은 검증 이후 테스트 자료는 제거하는 식으로 repo를 깔끔하게 유지하도록 하자."
- Windows development environment (mkdir 경로 구분자 주의)
- Git commit required at end of every session
- AGENTS.md must be updated at session end

CONTEXT FOR CONTINUATION
------------------------
- AGENTS.md (86KB) contains the complete project history, all design decisions, directory structure, and session logs. Always read it first in a new session.
- The project uses Budget tier (Gemini + GLM). Claude/GPT not configured.
- Voyage AI key not set — vector search (embeddings) won't work without it.
- Config at ~/.artemis/config.json — LSP is set to enabled=false in the saved config (needs manual enable).
- 15 competitive systems were analyzed. Full findings are in this session's chat history but key takeaways are in AGENTS.md design decisions.
- All 5 signature features from competitive analysis are now implemented.
- The training/ directory (Python) is complete code but has never been executed. User decided to defer.
