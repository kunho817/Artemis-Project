package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/config"
)

// Processor handles triage and auto-fix of GitHub issues.
type Processor struct {
	client    *Client
	store     IssueStore
	cfg       config.GitHubConfig
	logger    func(string)
	fixEngine FixEngine
	triageLLM TriageLLM
}

// TriageLLM sends a classification request to an LLM.
// Returns the raw response text. Injected to avoid github/ depending on llm/.
type TriageLLM func(ctx context.Context, systemPrompt, userPrompt string) (string, error)

const triagePrompt = `You are a GitHub issue triage classifier. Analyze the issue and classify it into exactly one category.

CATEGORIES:
- "auto_fix": Bug reports, error reports, crashes, broken functionality — issues describing specific code problems with enough detail for automated fixing.
- "needs_human": Security vulnerabilities, complex architectural changes, unclear/ambiguous requirements, insufficient description, or issues too risky for automated fixing.
- "not_applicable": Feature requests, enhancement suggestions, questions, help requests, documentation requests — anything that is NOT a code-fixing issue.

RULES:
- Security issues (CVE, vulnerability, exploit) → ALWAYS "needs_human"
- Feature requests / enhancements → ALWAYS "not_applicable"
- Questions / how-to requests → ALWAYS "not_applicable"
- Very short descriptions with no actionable detail → "needs_human"
- Clear bug reports with error messages or reproduction steps → "auto_fix"
- When uncertain, prefer "needs_human" over "auto_fix"

Respond with ONLY this JSON (no markdown, no code blocks, no extra text):
{"status": "auto_fix", "reason": "one sentence explanation"}`

type triageResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// NewProcessor creates a new issue processor.
func NewProcessor(cfg config.GitHubConfig, store IssueStore, logger func(string), fixEngine FixEngine, triageLLM TriageLLM) *Processor {
	if logger == nil {
		logger = func(string) {}
	}

	var client *Client
	if cfg.Token != "" {
		client = NewClient(cfg.Token)
	}

	return &Processor{
		client:    client,
		store:     store,
		cfg:       cfg,
		logger:    logger,
		fixEngine: fixEngine,
		triageLLM: triageLLM,
	}
}

// TriageIssue classifies an issue using LLM-first triage with heuristic fallback.
func (p *Processor) TriageIssue(ctx context.Context, issue *StoredIssue) (TriageStatus, string, error) {
	if issue == nil {
		return TriageNeedsHuman, "invalid issue payload", fmt.Errorf("triage: issue is nil")
	}
	if p.store == nil {
		return TriageNeedsHuman, "store unavailable", fmt.Errorf("triage: store not configured")
	}

	var status TriageStatus
	var reason string

	// Try LLM triage first.
	if p.triageLLM != nil {
		s, r, err := p.llmTriage(ctx, issue)
		if err == nil {
			status = s
			reason = fmt.Sprintf("[LLM] %s", r)
		} else {
			p.logger(fmt.Sprintf("LLM triage failed for #%d: %v, using heuristic", issue.IssueNumber, err))
		}
	}

	// Fallback to heuristic if LLM unavailable or failed.
	if status == "" {
		s, r, err := p.heuristicTriage(ctx, issue)
		if err != nil {
			return s, r, err
		}
		status = s
		reason = fmt.Sprintf("[heuristic] %s", r)
	}

	if err := p.store.UpdateTriageStatus(ctx, issue.IssueNumber, status, reason); err != nil {
		return status, reason, fmt.Errorf("triage: update status for issue #%d: %w", issue.IssueNumber, err)
	}

	return status, reason, nil
}

func (p *Processor) heuristicTriage(_ context.Context, issue *StoredIssue) (TriageStatus, string, error) {
	content := strings.ToLower(strings.TrimSpace(issue.Title + "\n" + issue.Body))
	bodyLen := len(strings.TrimSpace(issue.Body))

	switch {
	case containsAny(content, "feature request", "enhancement", "suggestion"):
		return TriageNotApplicable, "feature/enhancement request is not part of auto-fix pipeline", nil
	case containsAny(content, "question", "how to", "help"):
		return TriageNotApplicable, "question/help request is not a code-fix issue", nil
	case bodyLen < 30:
		return TriageNeedsHuman, "insufficient description", nil
	case containsAny(content, "security", "vulnerability", "cve"):
		return TriageNeedsHuman, "security issue requires manual review", nil
	case containsAny(content, "crash", "panic", "error", "bug", "fix", "broken", "fail"):
		return TriageAutoFix, "issue appears to be a bug/error suitable for automated scaffolding", nil
	default:
		return TriageNeedsHuman, "unable to determine fix approach", nil
	}
}

func (p *Processor) llmTriage(ctx context.Context, issue *StoredIssue) (TriageStatus, string, error) {
	triageCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	userPrompt := fmt.Sprintf("Issue #%d\nTitle: %s\nBody:\n%s", issue.IssueNumber, issue.Title, issue.Body)

	response, err := p.triageLLM(triageCtx, triagePrompt, userPrompt)
	if err != nil {
		return "", "", fmt.Errorf("LLM triage: %w", err)
	}

	response = strings.TrimSpace(response)
	// Strip markdown code blocks if present
	if strings.HasPrefix(response, "```") {
		lines := strings.Split(response, "\n")
		if len(lines) > 2 {
			lines = lines[1:]
			if strings.TrimSpace(lines[len(lines)-1]) == "```" {
				lines = lines[:len(lines)-1]
			}
			response = strings.Join(lines, "\n")
		}
	}

	var result triageResponse
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return "", "", fmt.Errorf("parse triage response: %w", err)
	}

	status := mapTriageStatus(result.Status)
	reason := result.Reason
	if reason == "" {
		reason = "LLM classification"
	}

	return status, reason, nil
}

func mapTriageStatus(s string) TriageStatus {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "auto_fix":
		return TriageAutoFix
	case "needs_human":
		return TriageNeedsHuman
	case "not_applicable":
		return TriageNotApplicable
	default:
		return TriageNeedsHuman
	}
}

// TriageAll triages all pending issues and returns status counts.
func (p *Processor) TriageAll(ctx context.Context) (int, int, int, error) {
	if p.store == nil {
		return 0, 0, 0, fmt.Errorf("triage all: store not configured")
	}

	pending, err := p.store.ListIssues(ctx, TriagePending, 200)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("triage all: list pending issues: %w", err)
	}

	autoFix := 0
	needsHuman := 0
	notApplicable := 0
	errCount := 0

	for _, issue := range pending {
		status, _, triageErr := p.TriageIssue(ctx, issue)
		if triageErr != nil {
			errCount++
			p.logger(fmt.Sprintf("Triage failed for issue #%d: %v", issue.IssueNumber, triageErr))
			continue
		}

		switch status {
		case TriageAutoFix:
			autoFix++
		case TriageNeedsHuman:
			needsHuman++
		case TriageNotApplicable:
			notApplicable++
		}
	}

	if errCount > 0 {
		return autoFix, needsHuman, notApplicable, fmt.Errorf("triage all: %d issues failed triage", errCount)
	}

	return autoFix, needsHuman, notApplicable, nil
}

// FixIssue scaffolds an auto-fix branch and draft PR for an issue.
func (p *Processor) FixIssue(ctx context.Context, issueNumber int) error {
	if p.client == nil {
		return fmt.Errorf("fix issue: github client not configured")
	}
	if p.store == nil {
		return fmt.Errorf("fix issue: store not configured")
	}
	if p.cfg.Owner == "" || p.cfg.Repo == "" {
		return fmt.Errorf("fix issue: owner/repo not configured")
	}

	issue, err := p.store.GetIssue(ctx, issueNumber)
	if err != nil {
		return fmt.Errorf("fix issue: load issue #%d: %w", issueNumber, err)
	}
	if issue == nil {
		return fmt.Errorf("fix issue: issue #%d not found", issueNumber)
	}

	fail := func(stepErr error) error {
		reason := stepErr.Error()
		if len(reason) > 500 {
			reason = reason[:500]
		}
		if uerr := p.store.UpdateTriageStatus(ctx, issueNumber, TriageFailed, reason); uerr != nil {
			p.logger(fmt.Sprintf("Fix issue #%d: failed to mark status failed: %v", issueNumber, uerr))
		}
		p.logger(fmt.Sprintf("Fix issue #%d failed: %v", issueNumber, stepErr))
		return stepErr
	}

	if err := p.store.UpdateTriageStatus(ctx, issueNumber, TriageInProgress, "running fix pipeline and creating draft PR"); err != nil {
		return fmt.Errorf("fix issue #%d: mark in_progress: %w", issueNumber, err)
	}

	branch := fmt.Sprintf("artemis/fix-%d", issueNumber)
	baseBranch := p.cfg.BaseBranch
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}

	// Try agent-driven fix if FixEngine is available.
	var fixResult *FixResult
	if p.fixEngine != nil {
		req := FixRequest{
			IssueNumber: issueNumber,
			Title:       issue.Title,
			Body:        issue.Body,
			Branch:      branch,
			BaseBranch:  baseBranch,
			Owner:       p.cfg.Owner,
			Repo:        p.cfg.Repo,
		}
		result, err := p.fixEngine.ExecuteFix(ctx, req)
		if err != nil {
			p.logger(fmt.Sprintf("FixEngine failed for issue #%d: %v, falling back to scaffold", issueNumber, err))
		} else {
			fixResult = result
		}
	}

	// If no FixEngine or no produced changes, create scaffold branch via API.
	if fixResult == nil || len(fixResult.FilesChanged) == 0 {
		if err := p.client.CreateBranch(ctx, p.cfg.Owner, p.cfg.Repo, baseBranch, branch); err != nil {
			return fail(fmt.Errorf("create branch %q: %w", branch, err))
		}
	}

	title := fmt.Sprintf("fix: resolve #%d — %s", issueNumber, strings.TrimSpace(issue.Title))
	var prBody string
	if fixResult != nil && len(fixResult.FilesChanged) > 0 {
		prBody = fmt.Sprintf("## Summary\n%s\n\nCloses #%d\n\n---\n*Generated by Artemis Agent*", fixResult.Summary, issueNumber)
	} else {
		bodyPreview := firstNRunes(strings.TrimSpace(issue.Body), 200)
		prBody = fmt.Sprintf(`## Summary
Automated fix scaffold for #%d.

**Issue**: %s
**Description**: %s

Code changes pending agent implementation.

Closes #%d

---
*Generated by Artemis Agent*`, issueNumber, issue.Title, bodyPreview, issueNumber)
	}

	pr, err := p.client.CreatePR(ctx, p.cfg.Owner, p.cfg.Repo, branch, baseBranch, title, prBody, true)
	if err != nil {
		return fail(fmt.Errorf("create draft PR: %w", err))
	}
	if pr == nil || pr.Number == nil {
		return fail(fmt.Errorf("create draft PR: missing PR number in response"))
	}
	prNumber := pr.GetNumber()

	comment := fmt.Sprintf("🤖 Artemis created a draft PR #%d to fix this issue.", prNumber)
	if err := p.client.CreateComment(ctx, p.cfg.Owner, p.cfg.Repo, issueNumber, comment); err != nil {
		return fail(fmt.Errorf("comment on issue #%d: %w", issueNumber, err))
	}

	if err := p.client.AddLabels(ctx, p.cfg.Owner, p.cfg.Repo, issueNumber, []string{"artemis-fix"}); err != nil {
		return fail(fmt.Errorf("add label to issue #%d: %w", issueNumber, err))
	}

	if err := p.store.UpdatePRNumber(ctx, issueNumber, prNumber); err != nil {
		return fail(fmt.Errorf("store PR number for issue #%d: %w", issueNumber, err))
	}

	if err := p.store.UpdateTriageStatus(ctx, issueNumber, TriageResolved, fmt.Sprintf("draft PR #%d created", prNumber)); err != nil {
		return fail(fmt.Errorf("mark issue #%d resolved: %w", issueNumber, err))
	}

	p.logger(fmt.Sprintf("Fix issue #%d: draft PR #%d created", issueNumber, prNumber))
	return nil
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func firstNRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
