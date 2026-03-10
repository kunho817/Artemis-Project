package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artemis-project/artemis/internal/config"
)

// TriageStatus represents the issue triage state.
type TriageStatus string

const (
	TriagePending       TriageStatus = "pending"
	TriageAutoFix       TriageStatus = "auto_fix"
	TriageNeedsHuman    TriageStatus = "needs_human"
	TriageNotApplicable TriageStatus = "not_applicable"
	TriageInProgress    TriageStatus = "in_progress"
	TriageResolved      TriageStatus = "resolved"
	TriageFailed        TriageStatus = "failed"
)

// StoredIssue is the persisted GitHub issue representation.
type StoredIssue struct {
	ID           int64
	IssueNumber  int
	Title        string
	Body         string
	State        string // "open", "closed"
	Labels       string // JSON array of label names
	Author       string
	TriageStatus TriageStatus
	TriageReason string // why this triage decision was made
	PRNumber     int    // linked PR number (0 if none)
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SyncedAt     time.Time
}

// IssueStore defines issue persistence used by the syncer.
type IssueStore interface {
	UpsertIssue(ctx context.Context, issue *StoredIssue) error
	GetIssue(ctx context.Context, issueNumber int) (*StoredIssue, error)
	ListIssues(ctx context.Context, status TriageStatus, limit int) ([]*StoredIssue, error)
	ListAllIssues(ctx context.Context, limit int) ([]*StoredIssue, error)
	UpdateTriageStatus(ctx context.Context, issueNumber int, status TriageStatus, reason string) error
	UpdatePRNumber(ctx context.Context, issueNumber int, prNumber int) error
	SaveComment(ctx context.Context, issueNumber int, commentID int64, body, author string, createdAt time.Time) error
}

// Syncer syncs GitHub issues into the local store.
type Syncer struct {
	client *Client
	store  IssueStore
	cfg    config.GitHubConfig
	stopCh chan struct{}
	logger func(string) // simple log callback for TUI messages
}

// NewSyncer creates a GitHub issue syncer.
func NewSyncer(cfg config.GitHubConfig, store IssueStore, logger func(string)) *Syncer {
	if logger == nil {
		logger = func(string) {}
	}

	var client *Client
	if cfg.Token != "" {
		client = NewClient(cfg.Token)
	}

	return &Syncer{
		client: client,
		store:  store,
		cfg:    cfg,
		stopCh: make(chan struct{}),
		logger: logger,
	}
}

// SyncOnce fetches all open issues and upserts them to the store.
func (s *Syncer) SyncOnce(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("github sync: client not configured")
	}
	if s.store == nil {
		return fmt.Errorf("github sync: store not configured")
	}
	if s.cfg.Owner == "" || s.cfg.Repo == "" {
		return fmt.Errorf("github sync: owner/repo not configured")
	}

	issues, err := s.client.ListOpenIssues(ctx, s.cfg.Owner, s.cfg.Repo, 1)
	if err != nil {
		return fmt.Errorf("github sync: list open issues: %w", err)
	}

	newCount := 0
	changedCount := 0
	for _, issue := range issues {
		if issue == nil {
			continue
		}

		labels := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			if label != nil && label.Name != nil {
				labels = append(labels, label.GetName())
			}
		}
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return fmt.Errorf("github sync: marshal labels for issue #%d: %w", issue.GetNumber(), err)
		}

		existing, err := s.store.GetIssue(ctx, issue.GetNumber())
		if err != nil {
			return fmt.Errorf("github sync: get stored issue #%d: %w", issue.GetNumber(), err)
		}

		triageStatus := TriagePending
		triageReason := ""
		prNumber := 0
		if existing != nil {
			triageStatus = existing.TriageStatus
			triageReason = existing.TriageReason
			prNumber = existing.PRNumber
		}

		stored := &StoredIssue{
			IssueNumber:  issue.GetNumber(),
			Title:        issue.GetTitle(),
			Body:         issue.GetBody(),
			State:        issue.GetState(),
			Labels:       string(labelsJSON),
			Author:       issue.GetUser().GetLogin(),
			TriageStatus: triageStatus,
			TriageReason: triageReason,
			PRNumber:     prNumber,
			CreatedAt:    issue.GetCreatedAt().Time,
			UpdatedAt:    issue.GetUpdatedAt().Time,
			SyncedAt:     time.Now(),
		}

		if err := s.store.UpsertIssue(ctx, stored); err != nil {
			return fmt.Errorf("github sync: upsert issue #%d: %w", issue.GetNumber(), err)
		}

		if existing == nil {
			newCount++
			s.logger(fmt.Sprintf("GitHub sync: new issue #%d %s", issue.GetNumber(), issue.GetTitle()))
			continue
		}

		if existing.Title != stored.Title || existing.Body != stored.Body || existing.State != stored.State || existing.Labels != stored.Labels || !existing.UpdatedAt.Equal(stored.UpdatedAt) {
			changedCount++
			s.logger(fmt.Sprintf("GitHub sync: updated issue #%d %s", issue.GetNumber(), issue.GetTitle()))
		}
	}

	s.logger(fmt.Sprintf("GitHub sync complete: %d issues (%d new, %d changed)", len(issues), newCount, changedCount))
	return nil
}

// Start begins periodic background syncing.
func (s *Syncer) Start(ctx context.Context) {
	go func() {
		if err := s.SyncOnce(ctx); err != nil {
			s.logger(fmt.Sprintf("GitHub sync failed: %v", err))
		}

		interval := s.cfg.PollInterval
		if interval <= 0 {
			interval = 5
		}

		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger("GitHub sync stopped (context cancelled)")
				return
			case <-s.stopCh:
				s.logger("GitHub sync stopped")
				return
			case <-ticker.C:
				if err := s.SyncOnce(ctx); err != nil {
					s.logger(fmt.Sprintf("GitHub sync failed: %v", err))
				}
			}
		}
	}()
}

// Stop signals the sync loop to stop.
func (s *Syncer) Stop() {
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
	}
}

// GetPendingReport returns a report of issues that need human triage.
func (s *Syncer) GetPendingReport(ctx context.Context) (string, error) {
	if s.store == nil {
		return "", fmt.Errorf("github sync: store not configured")
	}

	issues, err := s.store.ListIssues(ctx, TriageNeedsHuman, 100)
	if err != nil {
		return "", fmt.Errorf("github sync: list needs_human issues: %w", err)
	}

	if len(issues) == 0 {
		return "No issues require human triage.", nil
	}

	var b strings.Builder
	b.WriteString("Issues requiring human triage:\n")
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("- #%d %s\n", issue.IssueNumber, issue.Title))
		if issue.Author != "" {
			b.WriteString(fmt.Sprintf("  Author: %s\n", issue.Author))
		}
		if issue.TriageReason != "" {
			b.WriteString(fmt.Sprintf("  Reason: %s\n", issue.TriageReason))
		}
		b.WriteString(fmt.Sprintf("  Updated: %s\n", issue.UpdatedAt.Format(time.RFC3339)))
	}

	return strings.TrimSpace(b.String()), nil
}
