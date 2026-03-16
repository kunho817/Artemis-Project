# Artemis 설정 가이드

Artemis의 모든 설정은 `~/.artemis/config.json` 파일에서 관리됩니다. 이 문서는 각 설정 항목에 대한 상세 레퍼런스를 제공합니다.

---

## 1. 프로바이더 설정 (Providers)

LLM 서비스를 제공하는 프로바이더별 설정입니다.

### Claude (Anthropic)
- `api_key`: Anthropic API 키
- `endpoint`: API 엔드포인트 (기본값: `https://api.anthropic.com/v1/messages`)
- `model`: 사용할 모델명 (기본값: `claude-3-5-sonnet-20241022`)
- `enabled`: 활성화 여부

### Gemini (Google)
- `api_key`: Google AI Studio API 키
- `endpoint`: API 엔드포인트 (기본값: `https://generativelanguage.googleapis.com/v1beta/models`)
- `model`: 사용할 모델명 (기본값: `gemini-1.5-pro`)
- `enabled`: 활성화 여부

### GPT (OpenAI)
- `api_key`: OpenAI API 키
- `endpoint`: API 엔드포인트 (기본값: `https://api.openai.com/v1/chat/completions`)
- `model`: 사용할 모델명 (기본값: `gpt-4o`)
- `enabled`: 활성화 여부

### GLM (ZhipuAI)
- `api_key`: ZhipuAI API 키
- `endpoint`: API 엔드포인트 (기본값: `https://open.bigmodel.cn/api/paas/v4/chat/completions`)
- `model`: 사용할 모델명 (기본값: `glm-4-plus`)
- `enabled`: 활성화 여부
- **특이사항**: Coding Plan 전용 엔드포인트를 지원합니다.

### VLLM (Local)
- `endpoint`: 로컬 vLLM 서버 주소 (기본값: `http://localhost:8000/v1/chat/completions`)
- `model`: 로컬 모델명 (기본값: `qwen2.5-coder-7b`)
- `api_key`: 선택 사항 (로컬 서버 설정에 따름)
- `enabled`: 활성화 여부 (기본값: `false`, 사용자가 직접 활성화 필요)

---

## 2. 에이전트 설정 (Agents)

멀티 에이전트 파이프라인 및 역할별 설정을 관리합니다.

- `enabled`: 멀티 에이전트 모드 활성화 여부
- `tier`: 사용할 티어 선택 (`"premium"` 또는 `"budget"`)
- `premium` / `budget`: 13가지 역할별 프로바이더 매핑 설정
    - 역할 예시: `coder`, `analyzer`, `architect`, `tester`, `scout`, `consultant` 등
- `model_overrides`: 특정 역할에 대해 기본 모델 대신 사용할 모델을 지정합니다. (예: `{"scout": "gemini-3-flash-preview"}`)

---

## 3. 메모리 설정 (Memory)

대화 기록 및 추출된 사실 정보를 관리하는 SQLite 기반 메모리 시스템입니다.

- `enabled`: 메모리 시스템 활성화 여부
- `db_path`: SQLite 데이터베이스 파일 경로 (기본값: `~/.artemis/memory.db`)
- `consolidate_on_exit`: 세션 종료 시 자동 요약 및 사실 추출 수행 여부
- `fact_max_age_days`: 사실 정보의 유효 기간 (일 단위)
- `fact_min_use_count`: 사실 정보가 유지되기 위한 최소 사용 횟수

---

## 4. 벡터 검색 설정 (Vector)

시맨틱 검색을 위한 벡터 저장소 설정입니다.

- `enabled`: 벡터 검색 활성화 여부
- `provider`: 임베딩 프로바이더 (기본값: `voyage`)
- `api_key`: Voyage AI API 키
- `model`: 임베딩 모델명 (기본값: `voyage-code-3`)
- `store_path`: 벡터 데이터 저장 경로 (기본값: `~/.artemis/vectors/`)

---

## 5. Repo-map 설정 (Repo-map)

코드베이스의 구조를 파악하기 위한 인덱싱 설정입니다.

- `enabled`: 레포맵 활성화 여부
- `max_tokens`: 프롬프트에 주입할 레포맵의 최대 토큰 수 (기본값: `2048`)
- `update_on_write`: 파일 수정 시 실시간 인덱스 업데이트 여부
- `ctags_path`: Universal Ctags 바이너리 경로
- `exclude_patterns`: 인덱싱에서 제외할 파일 패턴 목록

---

## 6. LSP 설정 (LSP)

언어 서버 프로토콜(LSP)을 통한 코드 분석 및 도구 지원 설정입니다.

- `enabled`: LSP 기능 활성화 여부
- `auto_detect`: 설치된 LSP 서버 자동 감지 여부
- `servers`: 언어별 서버 상세 설정
    - `go`: `command`, `args`, `enabled` (기본: `gopls`)
    - `python`: `command`, `args`, `enabled` (기본: `pyright`)
    - `typescript`: `command`, `args`, `enabled` (기본: `typescript-language-server`)

---

## 7. 스킬 설정 (Skills)

에이전트의 기능을 확장하는 커스텀 스킬 설정입니다.

- `enabled`: 스킬 시스템 활성화 여부
- `global_dir`: 전역 스킬 파일 저장 디렉토리
- `auto_load`: 프로젝트 로컬 스킬 자동 로드 여부
- **커스텀 스킬 형식**: 마크다운 파일 상단에 YAML frontmatter를 사용하여 `description`과 적용될 파일 패턴(`globs`)을 정의합니다.

---

## 8. MCP 설정 (MCP)

Model Context Protocol(MCP) 서버 연결 설정입니다.

- `enabled`: MCP 활성화 여부
- `servers`: 연결할 MCP 서버 목록
    - 각 서버는 `id`, `command`, `args`, `env`, `enabled` 항목을 가집니다.
- **예시 (GitHub 서버)**:
  ```json
  {
    "id": "github",
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "your_token_here" },
    "enabled": true
  }
  ```

---

## 9. GitHub 설정 (GitHub)

GitHub 이슈 트래킹 및 자동 수정 연동 설정입니다.

- `enabled`: GitHub 연동 활성화 여부
- `token`: GitHub 개인 액세스 토큰 (PAT)
- `owner`: 저장소 소유자 (사용자명 또는 조직명)
- `repo`: 저장소 이름
- `poll_interval`: 이슈 동기화 주기 (분 단위)
- `auto_triage`: 새로운 이슈 발생 시 자동 분류 수행 여부
- `auto_fix`: 이슈에 대한 자동 수정 시도 여부
- `base_branch`: 작업의 기준이 되는 브랜치 (기본값: `main`)

---

## 10. 기타 설정

- `max_tool_iterations`: 에이전트가 한 번의 태스크에서 도구를 최대 몇 번까지 반복 호출할 수 있는지 지정합니다. (기본값: `20`)
- `theme`: TUI 테마를 선택합니다. (`default`, `dracula`, `tokyonight`)

---

## 11. 전체 기본 설정 예시

아래는 `config.json` 파일의 전체 구조 예시입니다.

```json
{
  "claude": {
    "api_key": "sk-ant-...",
    "enabled": true
  },
  "gemini": {
    "api_key": "AIza...",
    "enabled": true
  },
  "agents": {
    "enabled": true,
    "tier": "premium"
  },
  "memory": {
    "enabled": true,
    "consolidate_on_exit": true
  },
  "vector": {
    "enabled": true,
    "provider": "voyage",
    "api_key": "pa-..."
  },
  "repo_map": {
    "enabled": true,
    "max_tokens": 2048
  },
  "lsp": {
    "enabled": true,
    "auto_detect": true
  },
  "github": {
    "enabled": false,
    "owner": "your-org",
    "repo": "your-repo"
  },
  "theme": "tokyonight",
  "max_tool_iterations": 20
}
```
