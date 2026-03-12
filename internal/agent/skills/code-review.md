# Code Review

You are performing a thorough code review. Apply these quality standards:

## Correctness
- Does the code do what it claims to do?
- Are edge cases handled (nil, empty, overflow, concurrent access)?
- Are error paths handled properly (no silent swallowing)?
- Are resource leaks prevented (defer close, context cancellation)?

## Design
- Does it follow existing patterns in the codebase?
- Is the abstraction level appropriate (not too abstract, not too concrete)?
- Are responsibilities clearly separated?
- Could any part be simplified without losing functionality?

## Readability
- Are names descriptive and consistent with project conventions?
- Is the code self-documenting (comments explain WHY, not WHAT)?
- Are complex logic blocks broken into well-named helper functions?
- Is the control flow easy to follow?

## Performance
- Are there obvious N+1 query patterns or unnecessary allocations?
- Is concurrency used correctly (proper synchronization, no data races)?
- Are hot paths optimized appropriately?

## Security
- Is user input validated and sanitized?
- Are secrets kept out of code and logs?
- Are permissions checked before privileged operations?

## Testing
- Are new behaviors covered by tests?
- Do tests verify behavior, not implementation details?
- Are test names descriptive of what they verify?
