# Artemis Visual Testing Framework - Verification Plan

**Date:** 2026-03-22
**Status:** PHASE B & C IMPLEMENTED - CRITICAL BUGS DETECTED
**Plan Reference:** wobbly-orbiting-puzzle.md

---

## Executive Summary

The Artemis Project visual testing framework (Phase B & C) has been implemented but has **critical compilation errors** and **missing test files**. This document provides a structured verification plan to complete and validate the implementation.

### Current Status Summary

| Phase | Status | Completion | Blockers |
|-------|--------|------------|----------|
| Phase A (System Integration) | ✅ Complete | ~95% | E2E tests have failures |
| Phase B (Visual Framework) | ⚠️ Partial | ~70% | **COMPILATION ERROR** |
| Phase C (CI/CD) | ✅ Complete | ~100% | None |

---

## Completed Phases Checklist

### Phase A: System Integration (Completed)

| Task | File | Status | Notes |
|------|------|--------|-------|
| E2E Test Context | `tests/integration/e2e/context.go` | ✅ | EventCollector implemented |
| TUI Tests | `tests/integration/e2e/tui_test.go` | ✅ | 12 tests defined |
| Pipeline Tests | `tests/integration/e2e/pipeline_test.go` | ⚠️ | **FAILING** (timeout issues) |
| Agent Tests | `tests/integration/agent/orchestrator_test.go` | ✅ | Implemented |
| Shell Tests | `tests/integration/tools/shell_execution_test.go` | ✅ | Implemented |
| LSP Tests | `tests/integration/tools/lsp_test.go` | ✅ | Implemented |
| File Ops Tests | `tests/integration/tools/file_operations_test.go` | ✅ | Implemented |
| SQLite Tests | `tests/integration/memory/sqlite_test.go` | ✅ | **PASSING** (6/6) |
| Checkpoint Tests | `tests/integration/memory/checkpoint_test.go` | ✅ | **PASSING** (7/7) |
| Vector Tests | `tests/integration/memory/vector_test.go` | ✅ | **PASSING** |
| Consolidation | `tests/integration/memory/consolidation_test.go` | ✅ | Placeholder implemented |

### Phase B: Visual Testing Framework (Partial - Critical Bugs)

| Task | File | Status | Issues |
|------|------|--------|--------|
| Capture | `tests/integration/visual/capture.go` | ✅ | Implemented |
| Vision Router | `tests/integration/visual/vision_router.go` | ✅ | Implemented |
| Validators | `tests/integration/visual/validators.go` | ✅ | Implemented |
| Claude Vision | `tests/integration/visual/providers/claude_vision.go` | ✅ | Implemented |
| GPT Vision | `tests/integration/visual/providers/gpt_vision.go` | ✅ | Implemented |
| Gemini Vision | `tests/integration/visual/providers/gemini_vision.go` | ✅ | Implemented |
| Grid Prompts | `tests/integration/visual/prompts/grid_based.go` | ❌ | **COMPILATION ERROR** |
| CoT Prompts | `tests/integration/visual/prompts/chain_of_thought.go` | ✅ | Implemented |
| Spatial Prompts | `tests/integration/visual/prompts/spatial.go` | ✅ | Implemented |
| Scenarios | `tests/integration/visual/scenarios/*` | ❌ | **DIRECTORY MISSING** |
| Detector | `internal/vision/detector.go` | ✅ | YOLOv9 wrapper |
| UI Elements | `internal/vision/ui_elements.go` | ✅ | Defined |
| Layout Validator | `internal/vision/layout_validator.go` | ✅ | Implemented |
| Golden Master | `tests/integration/visual/regression/golden_master.go` | ✅ | Implemented |
| SSIM | `tests/integration/visual/regression/ssim.go` | ✅ | Implemented |
| Suite | `tests/integration/visual/regression/suite.go` | ✅ | Implemented |

### Phase C: CI/CD Integration (Complete)

| Task | File | Status | Notes |
|------|------|--------|-------|
| Integration Tests | `.github/workflows/integration-tests.yml` | ✅ | Multi-platform matrix |
| Visual Tests | `.github/workflows/visual-tests.yml` | ✅ | 6 job types defined |
| Coverage | `.github/workflows/coverage.yml` | ✅ | Implemented |
| Vision Config | `internal/vision/config.go` | ✅ | Implemented |
| Rate Limiter | `internal/vision/rate_limiter.go` | ✅ | Implemented |

---

## Critical Issues Detected

### 1. COMPILATION ERROR (CRITICAL)

**File:** `tests/integration/visual/prompts/grid_based.go`

**Errors:**
```
Line 105: fmt.Sprintf call needs 9 args but has 10 args
Line 230: fmt.Sprintf format %d reads arg #16, but call has 15 args
```

**Impact:** Visual testing framework cannot compile. All dependent tests are blocked.

**Fix Required:** Correct the fmt.Sprintf argument count mismatch.

---

## Files to Verify

### Core Framework Files (18 files)

```
tests/integration/visual/
├── capture.go              ✅ Verify: ModelViewCapturer CI-friendliness
├── vision.go               ✅ Verify: VisionProvider interface
├── vision_router.go        ✅ Verify: Fallback strategy
├── validators.go           ✅ Verify: Validation utilities
├── providers/
│   ├── claude_vision.go    ✅ Verify: Base64 encoding
│   ├── gpt_vision.go       ✅ Verify: API compatibility
│   └── gemini_vision.go    ✅ Verify: Response parsing
├── prompts/
│   ├── grid_based.go       ❌ FIX: Compilation errors
│   ├── chain_of_thought.go ✅ Verify: CoT templates
│   └── spatial.go          ✅ Verify: Spatial reasoning
└── regression/
    ├── golden_master.go    ✅ Verify: Version management
    ├── ssim.go             ✅ Verify: SSIM calculation accuracy
    └── suite.go            ✅ Verify: Test parallelization

internal/vision/
├── detector.go             ✅ Verify: YOLOv9 integration
├── ui_elements.go          ✅ Verify: Element type definitions
├── layout_validator.go     ✅ Verify: Hallucination detection
├── config.go               ✅ Verify: Provider configuration
└── rate_limiter.go         ✅ Verify: Rate limiting logic
```

### Missing Files (3)

```
tests/integration/visual/scenarios/
├── basic_scenarios.go      ❌ MISSING
├── complex_scenarios.go    ❌ MISSING
└── edge_cases.go           ❌ MISSING
```

---

## Verification Criteria

### 1. Build Verification

```bash
# Must succeed without errors
go build ./cmd/artemis

# Must compile all visual packages
go build ./tests/integration/visual/...
go build ./internal/vision/...
```

**Expected Result:** Clean build with zero errors.

### 2. Test Execution Verification

```bash
# E2E Tests (currently failing)
go test -v -timeout 10m ./tests/integration/e2e/...

# Memory Tests (currently passing)
go test -v ./tests/integration/memory/...

# Tools Tests
go test -v ./tests/integration/tools/...

# Visual Tests (blocked by compilation error)
go test -v ./tests/integration/visual/...
```

**Expected Result:**
- E2E: Fix timeout issues in `TestPipelineWithMockLLM`
- Memory: Continue passing (currently 13/13)
- Visual: After fix, should run without compilation errors

### 3. Integration Verification

| Component | Test | Expected Behavior |
|-----------|------|-------------------|
| Screen Capture | `ModelViewCapturer.CaptureModelView()` | Returns captured string |
| Vision Router | `VisionRouter.SelectProvider()` | Returns Claude first, falls back |
| Grid Builder | `GridPromptBuilder.BuildPrompt()` | Valid grid coordinates |
| YOLO Detector | `Detector.Detect()` | Returns detected elements |
| SSIM Calc | `SSIMCalculator.Calculate()` | Returns 0.0-1.0 similarity |
| Layout Validator | `LayoutValidator.ValidateLayout()` | PASS/FAIL/PARTIAL verdict |

### 4. CI/CD Verification

| Workflow | Trigger | Jobs | Status |
|----------|---------|------|--------|
| integration-tests.yml | push, PR | 4 jobs | ✅ Defined |
| visual-tests.yml | push, schedule | 6 jobs | ✅ Defined |
| coverage.yml | push | 1 job | ✅ Defined |

---

## Recommended Next Actions

### Priority 1: Fix Compilation Error (Critical)

1. **Fix `grid_based.go` fmt.Sprintf errors**
   - Line 105: Remove or add correct number of format arguments
   - Line 230: Fix argument count mismatch
   - Run `go build ./tests/integration/visual/prompts/` to verify

### Priority 2: Add Missing Test Files

2. **Create scenario test files**
   ```
   tests/integration/visual/scenarios/basic_scenarios.go
   tests/integration/visual/scenarios/complex_scenarios.go
   tests/integration/visual/scenarios/edge_cases.go
   ```

### Priority 3: Fix Failing E2E Tests

3. **Investigate `TestPipelineWithMockLLM` timeout**
   - Current: Times out after 10 minutes
   - Likely: Event synchronization issue or mock not responding

### Priority 4: Add Actual Test Functions

4. **Create `*_test.go` files in visual packages**
   - Current: No test files exist in `visual/` subdirectories
   - Need: Test files for capture, providers, prompts, regression

### Priority 5: Verify Vision Provider Integration

5. **Test Vision API calls**
   - Verify API key handling
   - Test fallback mechanism
   - Validate rate limiting

### Priority 6: Documentation

6. **Create usage documentation**
   - How to run visual tests locally
   - How to update golden masters
   - How to interpret validation results

---

## Test Execution Plan

### Step 1: Fix Compilation (5 min)
```bash
# Edit grid_based.go to fix fmt.Sprintf errors
# Verify fix
go build ./tests/integration/visual/prompts/...
```

### Step 2: Create Test Files (30 min)
```bash
# Create scenario files
# Add test functions to each visual package
```

### Step 3: Run Full Test Suite (15 min)
```bash
# Run all integration tests
go test -v -timeout 30m ./tests/integration/...
```

### Step 4: Verify CI/CD (10 min)
```bash
# Push to feature branch
# Verify GitHub Actions run successfully
```

---

## Success Metrics

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| Build Status | ❌ Error | ✅ Clean | Blocked |
| Test Coverage | ~20% | 70% | Pending |
| E2E Pass Rate | 66% (8/12) | 90%+ | Needs Fix |
| Visual Tests | 0% | 80%+ | Blocked |
| CI/CD Jobs | 100% | 100% | ✅ Pass |

---

## Open Questions

1. **YOLOv9 Model**: Where is the YOLOv9 model stored? How is it downloaded?
2. **API Keys**: Are test API keys configured in CI secrets?
3. **Golden Masters**: Should initial golden masters be created in CI or locally?
4. **Test Images**: Where should sample UI images for testing be stored?
5. **Timeout Values**: Are 10-minute test timeouts appropriate for CI?

---

## References

- Original Plan: `C:\Users\kunho\.claude\plans\wobbly-orbiting-puzzle.md`
- Test Harness: `tests/integration/harness/setup.go`
- Vision Provider Interface: `tests/integration/visual/vision.go`

---

**Verification Plan Version:** 1.0
**Next Review:** After compilation fix
**Owner:** Planner Agent
