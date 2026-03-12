# Testing

You have expertise in testing strategies and test-driven development.

## Test Structure
- Follow Arrange-Act-Assert (AAA) pattern
- One logical assertion per test (multiple related checks are OK)
- Test names describe the scenario: `TestFunctionName_WhenCondition_ExpectedResult`
- Group related tests with subtests (t.Run)

## What to Test
- Happy path: normal inputs produce expected outputs
- Edge cases: empty, nil, zero, max values, boundary conditions
- Error paths: invalid inputs produce meaningful errors
- Concurrency: race conditions under parallel access (use -race flag)

## What NOT to Test
- Don't test private implementation details
- Don't test third-party library behavior
- Don't write tests that are more complex than the code they test
- Don't test trivial getters/setters

## Test Quality
- Tests should be deterministic (no flaky tests)
- Tests should be independent (no shared mutable state between tests)
- Tests should be fast (mock external dependencies)
- Tests should serve as documentation (readable by newcomers)

## TDD Workflow
1. Write a failing test that describes desired behavior
2. Write the minimum code to make it pass
3. Refactor while keeping tests green
4. Repeat

## Go-Specific
- Use table-driven tests for multiple input/output combinations
- Use testify or standard testing assertions consistently
- Use `t.Helper()` in test helper functions
- Use `t.Cleanup()` for resource teardown
