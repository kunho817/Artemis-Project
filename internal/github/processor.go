package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/artemis-project/artemis/internal/config"
)

// Processor handles triage and auto-fix of GitHub issues.
type Processor struct {
	client *Client
	store  IssueStore
	cfg    config.GitHubConfig
	logger func(string)
}

// NewProcessor creates a new issue processor.
func NewProcessor(cfg config.GitHubConfig, store IssueStore, logger func(string)) *Processor {
	if logger == nil {
		logger = func(string) {}
	}

	var client *Client
	if cfg.Token != "" {
		client = NewClient(cfg.Token)
	}

	return &Processor{
		client: client,
		store:  store,
		cfg:    cfg,
		logger: logger,
	}
}

// TriageIssue classifies an issue using lightweight heuristics.
func (p *Processor) TriageIssue(ctx context.Context, issue *StoredIssue) (TriageStatus, string, error) {
	if issue == nil {
		return TriageNeedsHuman, "invalid issue payload", fmt.Errorf("triage: issue is nil")
	}
	if p.store == nil {
		return TriageNeedsHuman, "store unavailable", fmt.Errorf("triage: store not configured")
	}

	content := strings.ToLower(strings.TrimSpace(issue.Title + "\n" + issue.Body))
	bodyLen := len(strings.TrimSpace(issue.Body))

	status := TriageNeedsHuman
	reason := "unable to determine fix approach"

	switch {
	case containsAny(content, "feature request", "enhancement", "suggestion"):
		status = TriageNotApplicable
		reason = "feature/enhancement request is not part of auto-fix pipeline"
	case containsAny(content, "question", "how to", "help"):
		status = TriageNotApplicable
		reason = "question/help request is not a code-fix issue"
	case bodyLen < 30:
		status = TriageNeedsHuman
		reason = "insufficient description"
	case containsAny(content, "security", "vulnerability", "cve"):
		status = TriageNeedsHuman
		reason = "security issue requires manual review"
	case containsAny(content, "crash", "panic", "error", "bug", "fix", "broken", "fail"):
		status = TriageAutoFix
		reason = "issue appears to be a bug/error suitable for automated scaffolding"
	}

	if err := p.store.UpdateTriageStatus(ctx, issue.IssueNumber, status, reason); err != nil {
		return status, reason, fmt.Errorf("triage: update status for issue #%d: %w", issue.IssueNumber, err)
	}

	return status, reason, nil
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

	if err := p.store.UpdateTriageStatus(ctx, issueNumber, TriageInProgress, "creating branch and draft PR scaffold"); err != nil {
		return fmt.Errorf("fix issue #%d: mark in_progress: %w", issueNumber, err)
	}

	branch := fmt.Sprintf("artemis/fix-%d", issueNumber)
	baseBranch := p.cfg.BaseBranch
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}

	if err := p.client.CreateBranch(ctx, p.cfg.Owner, p.cfg.Repo, baseBranch, branch); err != nil {
		return fail(fmt.Errorf("create branch %q: %w", branch, err))
	}

	title := fmt.Sprintf("fix: resolve #%d — %s", issueNumber, strings.TrimSpace(issue.Title))
	bodyPreview := firstNRunes(strings.TrimSpace(issue.Body), 200)
	prBody := fmt.Sprintf(`## Summary
Automated fix scaffold for #%d.

**Issue**: %s
**Description**: %s

Code changes pending agent implementation.

Closes #%d

---
*Generated by Artemis Agent*`, issueNumber, issue.Title, bodyPreview, issueNumber)

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
