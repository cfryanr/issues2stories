package githubapi

import (
	"context"
	"fmt"
	"golang.org/x/oauth2"
	"net/url"
	"reflect"

	"github.com/google/go-github/v33/github"
	"github.com/google/go-querystring/query"
	"issues2stories/internal/importtypes"
)

// A simplified interface of the bigger github.Client API.
type GitHubAPI interface {
	// Get the interesting details of an issue. Ignore the uninteresting parts.
	GetIssue(ctx context.Context, issueNumber int) (*Issue, error)

	// Overwrite requested fields of the issue in a PATCH-style update.
	// See https://docs.github.com/en/rest/reference/issues#update-an-issue for details.
	UpdateIssue(ctx context.Context, issueNumber int, updates *github.IssueRequest) error

	// List all open issues in a custom format. Internally reads all pages of GitHub's paginated results.
	ListAllOpenIssuesForRepoInImportFormat(ctx context.Context) ([]importtypes.Issue, error)
}

// A simplified version of the bigger github.Issue type.
type Issue struct {
	Labels []string
}

type gitHubClient struct {
	org, repo string
	client    *github.Client
}

func New(apiToken, org, repo string) GitHubAPI {
	token := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: apiToken})
	tokenClient := oauth2.NewClient(context.Background(), token)
	return &gitHubClient{org: org, repo: repo, client: github.NewClient(tokenClient)}
}

// Thin wrapper around github.IssuesService's GetIssue() to only return what we need.
func (c *gitHubClient) GetIssue(ctx context.Context, issueNumber int) (*Issue, error) {
	// See https://docs.github.com/en/rest/reference/issues#get-an-issue
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

// Thin wrapper around github.IssuesService's UpdateIssue().
func (c *gitHubClient) UpdateIssue(ctx context.Context, issueNumber int, updates *github.IssueRequest) error {
	// See https://docs.github.com/en/rest/reference/issues#update-an-issue
	_, _, err := c.client.Issues.Edit(ctx, c.org, c.repo, issueNumber, updates)
	return err
}

// List all open issues in the repository.
// Follow the GitHub API pagination until the end to read all results, and return a custom format tailored to our needs.
func (c *gitHubClient) ListAllOpenIssuesForRepoInImportFormat(ctx context.Context) ([]importtypes.Issue, error) {
	// See https://docs.github.com/en/rest/reference/issues#list-repository-issues
	opt := &github.IssueListByRepoOptions{
		State:       "open",
		Sort:        "created",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100}, // 100 is the max allowed according to GitHub API docs
	}
	var allIssues []importtypes.Issue
	for {
		pageOfIssues, resp, err := c.getOnePageOfListAllIssuesForRepo(ctx, opt)
		if err != nil {
			return nil, err
		}
		allIssues = append(allIssues, pageOfIssues...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return allIssues, nil
}

// This is mostly a copy of github.IssuesService's ListByRepo(), but we deserialize into a custom struct
// to make it more convenient for our needs and to avoid the runtime/space penalty of deserializing
// the majority of the json response content.
func (c *gitHubClient) getOnePageOfListAllIssuesForRepo(ctx context.Context, opts *github.IssueListByRepoOptions) ([]importtypes.Issue, *github.Response, error) {
	// See https://docs.github.com/en/rest/reference/issues#list-repository-issues
	u := fmt.Sprintf("repos/%v/%v/issues", c.org, c.repo)
	u, err := addOptions(u, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := c.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	var issues []importtypes.Issue
	resp, err := c.client.Do(ctx, req, &issues)
	if err != nil {
		return nil, resp, err
	}

	return issues, resp, nil
}

// This is a copy of the private function github.addOptions() so we can use it in our method above.
func addOptions(s string, opts interface{}) (string, error) {
	v := reflect.ValueOf(opts)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return s, nil
	}

	u, err := url.Parse(s)
	if err != nil {
		return s, err
	}

	qs, err := query.Values(opts)
	if err != nil {
		return s, err
	}

	u.RawQuery = qs.Encode()
	return u.String(), nil
}
