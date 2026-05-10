# Getting Started

This guide describes the current Artemis Python/FastAPI/React implementation.
The legacy Go TUI is preserved on `legacy/go-tui` and is not the default path.

## 1. Prepare Python

From the repository root:

```powershell
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install --upgrade pip
.\.venv\Scripts\python.exe -m pip install fastapi annotated-doc uvicorn langchain langchain-openai langgraph langsmith pydantic python-dotenv httpx
```

## 2. Prepare The GUI

```powershell
cd apps\gui
npm install
```

## 3. Configure Environment

Copy the example file when live GLM calls are needed:

```powershell
Copy-Item .env.example .env
```

Minimum useful settings:

```text
ZAI_API_KEY=
ZAI_BASE_URL=https://api.z.ai/api/coding/paas/v4
ZAI_MODEL=glm-5.1
VITE_CONTROL_PLANE_URL=http://127.0.0.1:8000
```

If `ZAI_API_KEY` is empty, Work Package generation uses the deterministic
fallback path and records the fallback reason in events.

## 4. Start Services

```powershell
.\.venv\Scripts\python.exe scripts\start_mvp2_services.py
```

This starts:

- Agent Backend at `http://127.0.0.1:8765`
- Control Plane at `http://127.0.0.1:8000`

## 5. Start GUI

In another shell:

```powershell
cd apps\gui
npm run dev
```

Open the printed Vite URL, normally `http://127.0.0.1:5173`.

## 6. First Flow

1. Open a project root.
2. Create a session.
3. Submit a Work Package request.
4. Review events, trace, artifacts, and approval.
5. Approve the Work Package.
6. Create an ImplementationRun.
7. Review and approve the PatchSet.
8. Apply the PatchSet only after approval.
9. Review verification and ReviewResult.
10. Promote useful decisions, failures, or summaries to memory.
11. Run Risk Radar again.

The Command Center at the top of the GUI summarizes pending approvals, open
risks, selected memory, recent runs, failed runs, and the next recommended
action.

## 7. Verify The Local Install

Quick Alpha verification:

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile quick
```

Full Alpha verification:

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile full
```

The full profile includes MVP 2 through MVP 6 GUI smoke tests and can take
longer because it starts isolated backend and GUI processes.
