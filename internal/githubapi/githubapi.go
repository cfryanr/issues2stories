package githubapi

import (
	"context"
	"os"

	"github.com/google/go-github/v33/github"
	"golang.org/x/oauth2"
)

// A simplified interface of the bigger github.Client API.
type GitHubAPI interface {
	// Get the interesting details of an issue. Ignore the uninteresting parts.
	GetIssue(ctx context.Context, issueNumber int) (*Issue, error)

	// Overwrite requested fields of the issue in a PATCH-style update.
	// See https://docs.github.com/en/rest/reference/issues#update-an-issue for details.
	UpdateIssue(ctx context.Context, issueNumber int, updates *github.IssueRequest) error
}

// A simplified version of the bigger github.Issue type.
type Issue struct {
	Labels []string
}

type gitHubClient struct {
	org, repo string
	client    *github.Client
}

func New() GitHubAPI {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: os.Getenv("GITHUB_API_TOKEN")})
	tokenClient := oauth2.NewClient(context.Background(), token)
	return &gitHubClient{
		org:    os.Getenv("GITHUB_ORG"),
		repo:   os.Getenv("GITHUB_REPO"),
		client: github.NewClient(tokenClient),
	}
}

func (c *gitHubClient) GetIssue(ctx context.Context, issueNumber int) (*Issue, error) {
	issue, _, err := c.client.Issues.Get(ctx, c.org, c.repo, issueNumber)
	if err != nil {
		return nil, err
	}
	labels := []string{}
	for _, label := range issue.Labels {
		labels = append(labels, *label.Name)
	}
	return &Issue{Labels: labels}, nil
}

func (c *gitHubClient) UpdateIssue(ctx context.Context, issueNumber int, updates *github.IssueRequest) error {
	_, _, err := c.client.Issues.Edit(ctx, c.org, c.repo, issueNumber, updates)
	return err
}
