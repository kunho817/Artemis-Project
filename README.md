# Artemis

**TUI-based multi-agent coding system** powered by multiple LLM providers.

Artemis orchestrates specialized AI agents (Coder, Analyzer, Architect, Tester, etc.) through an intelligent pipeline to handle complex software engineering tasks — from code generation to refactoring to bug fixing.

## Features

- **Multi-Agent Orchestration** — 13 specialized agents with dynamic task planning
- **5 LLM Providers** — Claude, Gemini, GPT, GLM, vLLM (local)
- **22+ Tools** — File I/O, grep, git, LSP, test runner, dependency analysis, AST search/replace
- **LSP Integration** — Real-time diagnostics, go-to-definition, find-references, safe rename
- **MCP Support** — Connect external MCP servers for unlimited tool expansion
- **Persistent Memory** — SQLite + FTS5 + vector search, session history, fact tracking
- **TUI Interface** — Single/Split panel, glamour markdown, overlays, dark themes
- **Autonomous Mode** — Verify-gated execution loops (build/test verification)
- **Failure Recovery** — 3-stage recovery: retry → consultant diagnosis → user decision
- **Checkpoint/Resume** — Pipeline execution survives interruptions
- **Custom Skills** — Project-specific or global skills via markdown + YAML frontmatter

## Quick Start

```bash
# Build
go build -o artemis ./cmd/artemis/

# Run
./artemis
```

On first launch, press `Ctrl+S` to open Settings and configure at least one LLM provider API key.

## Requirements

- **Go 1.21+**
- **One or more LLM API keys**: Gemini, Claude, GPT, or GLM
- **Optional**: gopls (Go LSP), ast-grep (AST tools), Universal Ctags (repo-map)

## Configuration

Config is stored at `~/.artemis/config.json`. Edit via TUI (`Ctrl+S`) or directly.

```json
{
  "gemini": {
    "api_key": "your-key",
    "model": "gemini-3.1-pro-preview",
    "enabled": true
  },
  "agents": {
    "enabled": true,
    "tier": "budget"
  }
}
```

See [Configuration Guide](docs/configuration.md) for all options.

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+Enter` | Send message |
| `Ctrl+S` | Settings |
| `Ctrl+K` | Command Palette |
| `Ctrl+A` | Agent Selector |
| `Ctrl+O` | File Picker |
| `Ctrl+D` | Git Diff Viewer |
| `Ctrl+L` | Clear screen |
| `Ctrl+C` | Quit |

## Architecture

Artemis uses a phased pipeline architecture:

```
User Input → Orchestrator (intent classification)
  → trivial:  Direct agent response
  → complex:  ExecutionPlan → Multi-agent pipeline
                ├── Step 1: [Analyzer] parallel with [Scout]
                ├── Step 2: [Coder] with tools + autonomous verify
                └── Step 3: [QA] review → feedback loop
```

See [Architecture Guide](docs/architecture.md) for details.

## Project Structure

```
cmd/artemis/          Entry point
internal/
  agent/              Agent system (13 roles, skills, categories)
  bus/                Event bus (agent → TUI communication)
  config/             Configuration management
  github/             GitHub issue tracker integration
  llm/                LLM providers (Claude, Gemini, GPT, GLM, vLLM)
  lsp/                Language Server Protocol client
  mcp/                Model Context Protocol client
  memory/             Persistent memory (SQLite, vector search, repo-map)
  orchestrator/       Pipeline engine, execution plans, recovery
  state/              Session state, checkpoints
  tools/              22+ agent tools
  tui/                Terminal UI (bubbletea)
training/             Code generation model training pipeline
```

## Slash Commands

| Command | Description |
|---------|-------------|
| `/sessions` | List previous sessions |
| `/load <id>` | Load a previous session |
| `/fix #<n>` | Auto-fix a GitHub issue |
| `/issues` | Show GitHub issues |
| `/undo` | Undo last auto-commit |
| `/help` | Show available commands |

## Themes

Built-in: `default`, `dracula`, `tokyonight`

Custom themes: Export via Command Palette → edit JSON → place in `~/.artemis/themes/`

## Custom Skills

Create `.md` files with YAML frontmatter:

```markdown
---
description: "React component conventions"
globs: ["*.tsx", "src/components/**"]
---
# React Rules
- Use functional components with hooks
- Props interface above component
```

Place in:
- `~/.artemis/skills/` — Global (all projects)
- `.artemis/skills/` — Project-specific (higher priority)

## License

MIT
