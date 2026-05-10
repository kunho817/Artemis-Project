# Alpha Dogfooding Runbook

Purpose: prove that Artemis can manage an Artemis improvement through the
current MVP 1-6 surface without hidden automation.

## Scenario

Use Artemis against this repository or an isolated copy:

1. Open the Artemis project in the GUI.
2. Create an Alpha dogfooding session.
3. Run Risk Radar with selected memory enabled.
4. Accept one relevant RiskFinding.
5. Convert the finding to a pending-approval WorkPackage.
6. Approve the WorkPackage.
7. Create an ImplementationRun.
8. Review the ImplementationPlan and PatchSet.
9. Approve the PatchSet.
10. Apply the PatchSet.
11. Run or inspect VerificationRun.
12. Inspect ReviewResult.
13. Create a session summary memory or promote a useful failure/decision.
14. Run Risk Radar again.
15. Check the Command Center next action.

## Automated Smoke

The in-process smoke follows the same shape on a temporary project:

```powershell
.\.venv\Scripts\python.exe scripts\smoke_alpha_dogfood.py
```

The smoke creates a temporary project, selected memory rule, risk scan,
converted WorkPackage, approved ImplementationRun, approved PatchSet, local
verification, review result, memory summary, rescan, and Command Center query.

## Expected Evidence

- `risk_scan.completed`
- `risk_finding.accepted`
- `risk_finding.converted_to_work_package`
- `approval.approved`
- `implementation_plan.created`
- `patch_set.pending_approval`
- `patch_set.approved`
- `patch_set.applied`
- `verification.completed` or a visible blocked/failed result
- `review.completed`
- `memory.item.created`
- a later RiskScan result
- Command Center next action payload

## Notes

Live GLM output is not required for this smoke. LLM structured WorkPackage
generation has separate contract coverage and is enabled when `ZAI_API_KEY` is
configured.
