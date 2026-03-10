package github

import (
	"context"
	"errors"
	"fmt"
	"time"

	gh "github.com/google/go-github/v66/github"
)

// Client wraps go-github client functionality for Artemis.
type Client struct {
	api *gh.Client
}

// NewClient creates a new authenticated GitHub client.
func NewClient(token string) *Client {
	return &Client{api: gh.NewClient(nil).WithAuthToken(token)}
}

// ListOpenIssues lists all open issues (non-PRs), handling pagination.
func (c *Client) ListOpenIssues(ctx context.Context, owner, repo string, page int) ([]*gh.Issue, error) {
	if page <= 0 {
		page = 1
	}

	all := make([]*gh.Issue, 0, 64)
	opts := &gh.IssueListByRepoOptions{
		State:       "open",
		ListOptions: gh.ListOptions{Page: page, PerPage: 50},
	}

	for {
		issues, resp, err := c.api.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, wrapGitHubError("list open issues", err)
		}

		for _, issue := range issues {
			if issue != nil && issue.PullRequestLinks == nil {
				all = append(all, issue)
			}
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

// GetIssue gets a single issue by number.
func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (*gh.Issue, error) {
	issue, _, err := c.api.Issues.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, wrapGitHubError(fmt.Sprintf("get issue #%d", number), err)
	}
	return issue, nil
}

// CreateComment creates an issue comment.
func (c *Client) CreateComment(ctx context.Context, owner, repo string, number int, body string) error {
	_, _, err := c.api.Issues.CreateComment(ctx, owner, repo, number, &gh.IssueComment{Body: &body})
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("create comment on issue #%d", number), err)
	}
	return nil
}

// CloseIssue closes an issue.
func (c *Client) CloseIssue(ctx context.Context, owner, repo string, number int) error {
	state := "closed"
	_, _, err := c.api.Issues.Edit(ctx, owner, repo, number, &gh.IssueRequest{State: &state})
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("close issue #%d", number), err)
	}
	return nil
}

// AddLabels adds labels to an issue.
func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	_, _, err := c.api.Issues.AddLabelsToIssue(ctx, owner, repo, number, labels)
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("add labels to issue #%d", number), err)
	}
	return nil
}

// RemoveLabel removes a single label from an issue.
func (c *Client) RemoveLabel(ctx context.Context, owner, repo string, number int, label string) error {
	_, err := c.api.Issues.RemoveLabelForIssue(ctx, owner, repo, number, label)
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("remove label %q from issue #%d", label, number), err)
	}
	return nil
}

// CreateBranch creates a new branch from the base branch HEAD.
func (c *Client) CreateBranch(ctx context.Context, owner, repo, baseBranch, newBranch string) error {
	baseRefName := "refs/heads/" + baseBranch
	baseRef, _, err := c.api.Git.GetRef(ctx, owner, repo, baseRefName)
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("get base ref %q", baseRefName), err)
	}

	if baseRef == nil || baseRef.Object == nil || baseRef.Object.SHA == nil {
		return fmt.Errorf("create branch %q: base ref %q missing SHA", newBranch, baseRefName)
	}

	newRef := &gh.Reference{
		Ref: stringPtr("refs/heads/" + newBranch),
		Object: &gh.GitObject{
			SHA: baseRef.Object.SHA,
		},
	}

	_, _, err = c.api.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		return wrapGitHubError(fmt.Sprintf("create branch %q", newBranch), err)
	}

	return nil
}

// CreatePR creates a pull request.
func (c *Client) CreatePR(ctx context.Context, owner, repo, head, base, title, body string, draft bool) (*gh.PullRequest, error) {
	pr, _, err := c.api.PullRequests.Create(ctx, owner, repo, &gh.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
		Body:  &body,
		Draft: &draft,
	})
	if err != nil {
		return nil, wrapGitHubError("create pull request", err)
	}
	return pr, nil
}

func wrapGitHubError(op string, err error) error {
	var rlErr *gh.RateLimitError
	if errors.As(err, &rlErr) {
		reset := "unknown"
		if !rlErr.Rate.Reset.Time.IsZero() {
			reset = rlErr.Rate.Reset.Time.Format(time.RFC3339)
		}
		return fmt.Errorf("github: %s: rate limited (reset at %s): %w", op, reset, err)
	}

	var abuseErr *gh.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		retry := "unknown"
		if abuseErr.RetryAfter != nil {
			retry = abuseErr.RetryAfter.String()
		}
		return fmt.Errorf("github: %s: abuse rate limit (retry after %s): %w", op, retry, err)
	}

	return fmt.Errorf("github: %s: %w", op, err)
}

func stringPtr(v string) *string { return &v }
