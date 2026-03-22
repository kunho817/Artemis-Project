## [Visual Testing Framework] - 2026-03-22

- [ ] YOLOv9 Model Storage: Where should the YOLOv9 model file be stored? Is it downloaded at runtime or bundled?
- [ ] Test API Keys: Are Vision API keys configured in GitHub Secrets for CI/CD?
- [ ] Golden Master Creation: Should initial golden master images be created in CI or locally by developers?
- [ ] Test Images Location: Where should sample UI images for testing be stored in the repo?
- [ ] Test Timeout Values: Are 10-minute timeouts appropriate for E2E tests, or should they be reduced?
- [ ] SSIM Threshold: What is the appropriate SSIM threshold for visual regression (currently 0.95)?
- [ ] YOLO Python Dep: Should Python/ultralytics be a CI dependency or optional for local testing only?
- [ ] Vision Test Data: Should we create synthetic UI test images or capture from real TUI sessions?
- [ ] Fallback Strategy: Which provider should be the primary fallback when Claude Vision fails?
- [ ] Grid Builder Args: The `grid_based.go` has argument count issues - what should the correct prompt format be?

## [Wobbly Orbiting Puzzle - Original Plan] - 2025-03-21

- [ ] Phase A E2E Tests: Some pipeline tests are timing out after 10 minutes - needs investigation
- [ ] Phase B Scenarios: The `tests/integration/visual/scenarios/` directory is missing - needs creation
