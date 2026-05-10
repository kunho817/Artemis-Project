# Alpha Verification Matrix

Use `scripts/verify_alpha.py` as the canonical entry point.

## Quick Profile

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile quick
```

Checks:

| Check | Command |
| --- | --- |
| compileall | `python -m compileall services tests scripts` |
| unittest | `python -m unittest discover -s tests` |
| FastAPI smoke | `python scripts/smoke_api.py` |
| GUI build | `npm run build` in `apps/gui` |
| npm audit | `npm audit --omit=dev` in `apps/gui` |
| Alpha dogfood smoke | `python scripts/smoke_alpha_dogfood.py` |

## Full Profile

```powershell
.\.venv\Scripts\python.exe scripts\verify_alpha.py --profile full
```

The full profile runs all quick checks plus:

| Check | Command |
| --- | --- |
| MVP 2 GUI smoke | `python scripts/smoke_mvp2_gui.py` |
| MVP 3 GUI smoke | `python scripts/smoke_mvp3_gui.py` |
| MVP 4 GUI smoke | `python scripts/smoke_mvp4_gui.py` |
| MVP 5 GUI smoke | `python scripts/smoke_mvp5_gui.py` |
| MVP 6 GUI smoke | `python scripts/smoke_mvp6_gui.py` |

## Failure Triage

- Compile or unittest failure means the Python service boundary is broken.
- FastAPI smoke failure means the HTTP Control Plane / Agent Backend boundary or
  Alpha schema/Command Center endpoint is broken.
- GUI build failure means TypeScript or Vite integration is broken.
- Alpha dogfood failure means the end-to-end operating workflow is broken.
- MVP GUI smoke failure identifies the affected MVP surface by script name.

## Current Alpha Additions

- `work_package.generation_path` distinguishes `llm_structured` from
  `deterministic_fallback`.
- `GET /api/storage/schema` exposes schema version and migration records.
- `GET /api/projects/{project_id}/command-center` exposes pending approvals,
  recent runs, top risks, selected memory, quality snapshot, and next action.
