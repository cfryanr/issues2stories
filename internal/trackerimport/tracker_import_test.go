package trackerimport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-github/v33/github"
	"github.com/stretchr/testify/require"
	"issues2stories/internal/config"
	"issues2stories/internal/githubapi"
	"issues2stories/internal/importtypes"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := ioutil.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return string(content)
}

// Parse the GitHub list issues API response the same way that our production
// code implementation of ListAllOpenIssuesForRepoInImportFormat() would do it.
// The goal here is to test that we can go all the way from GitHub API JSON to
// Tracker import API XML, and that we get the exact expected XML.
func parseIssuesListJson(t *testing.T, issuesJson string) []importtypes.Issue {
	var issues []importtypes.Issue
	err := json.NewDecoder(strings.NewReader(issuesJson)).Decode(&issues)
	require.NoError(t, err)
	return issues
}

type fakeGitHubListIssuesReturnValues struct {
	issueLists [][]importtypes.Issue
	errors     []error
}

type fakeGitHubListIssuesActivity struct {
	invocations int
}

type fakeGitHubListIssues struct {
	returns *fakeGitHubListIssuesReturnValues
	actual  *fakeGitHubListIssuesActivity
}

type fakeGitHubAPI struct {
	listIssues *fakeGitHubListIssues
}

func (f *fakeGitHubAPI) GetIssue(_ context.Context, _ int) (*githubapi.Issue, error) {
	panic("not used by the test subject")
}

func (f *fakeGitHubAPI) UpdateIssue(_ context.Context, _ int, _ *github.IssueRequest) error {
	panic("not used by the test subject")
}

func (f *fakeGitHubAPI) ListAllOpenIssuesForRepoInImportFormat(_ context.Context) ([]importtypes.Issue, error) {
	thisCall := f.listIssues.actual.invocations
	f.listIssues.actual.invocations++
	if f.listIssues.returns != nil && f.listIssues.returns.errors != nil && f.listIssues.returns.errors[thisCall] != nil {
		return nil, f.listIssues.returns.errors[thisCall]
	}
	return f.listIssues.returns.issueLists[thisCall], nil
}

func TestHandleTrackerImport(t *testing.T) {
	tests := []struct {
		name string

		method      string
		requestAuth *config.BasicAuthCredentials

		wantStatus      int
		wantBody        string
		wantContentType string

		gitHubListIssuesReturns         *fakeGitHubListIssuesReturnValues
		wantGitHubListIssuesInvocations *fakeGitHubListIssuesActivity
	}{
		{
			name:            "wrong method is an error",
			requestAuth:     &config.BasicAuthCredentials{Username: "correct-username", Password: "correct-password"},
			method:          http.MethodPost,
			wantStatus:      http.StatusMethodNotAllowed,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Request method is not supported: POST\n",
		},
		{
			name:            "wrong username is an error",
			requestAuth:     &config.BasicAuthCredentials{Username: "wrong", Password: "correct-password"},
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Unauthorized\n",
		},
		{
			name:            "wrong password is an error",
			requestAuth:     &config.BasicAuthCredentials{Username: "correct-username", Password: "wrong"},
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Unauthorized\n",
		},
		{
			name:            "missing auth on request is an error",
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Unauthorized\n",
		},
		{
			name:        "asking GitHub for the list of issues fails",
			requestAuth: &config.BasicAuthCredentials{Username: "correct-username", Password: "correct-password"},
			gitHubListIssuesReturns: &fakeGitHubListIssuesReturnValues{
				errors: []error{fmt.Errorf("fake error from GitHub")},
			},
			wantGitHubListIssuesInvocations: &fakeGitHubListIssuesActivity{
				invocations: 1,
			},
			wantStatus:      http.StatusBadGateway,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "failed to get issues from GitHub API\n",
		},
		{
			name:        "happy path",
			requestAuth: &config.BasicAuthCredentials{Username: "correct-username", Password: "correct-password"},
			gitHubListIssuesReturns: &fakeGitHubListIssuesReturnValues{
				issueLists: [][]importtypes.Issue{
					// This test fixture includes an issue with several labels, an issue which is labeled "bug",
					// an issue with no labels, issues that are not labeled "bug", and a pull request issue.
					// This is representative enough to test all the interesting cases for the current
					// business logic.
					parseIssuesListJson(t, readFixture(t, "github_list_issues_response1.json")),
				},
			},
			wantGitHubListIssuesInvocations: &fakeGitHubListIssuesActivity{
				invocations: 1,
			},
			wantStatus:      http.StatusOK,
			wantContentType: "text/xml; charset=utf-8",
			wantBody:        strings.TrimSpace(readFixture(t, "expected_tracker_import_response_body1.xml")),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gitHubAPI := fakeGitHubAPI{
				listIssues: &fakeGitHubListIssues{
					returns: test.gitHubListIssuesReturns,
					actual:  &fakeGitHubListIssuesActivity{},
				},
			}
			if test.wantGitHubListIssuesInvocations == nil {
				test.wantGitHubListIssuesInvocations = &fakeGitHubListIssuesActivity{}
			}

			if test.method == "" {
				test.method = http.MethodGet
			}

			configuredAuth := &config.BasicAuthCredentials{Username: "correct-username", Password: "correct-password"}

			subject := NewHandler(&gitHubAPI, configuredAuth)

			req := httptest.NewRequest(test.method, "/some/path", nil)
			if test.requestAuth != nil {
				basicAuthHeaderValue := "Basic " + base64.StdEncoding.EncodeToString(
					[]byte((test.requestAuth.Username + ":" + test.requestAuth.Password)),
				)
				req.Header.Add("Authorization", basicAuthHeaderValue)
			}

			rsp := httptest.NewRecorder()

			subject.ServeHTTP(rsp, req)

			require.Equal(t, test.wantGitHubListIssuesInvocations.invocations, gitHubAPI.listIssues.actual.invocations, "wrong number of GitHub ListAllOpenIssuesForRepoInImportFormat() API invocations")

			require.Equal(t, test.wantStatus, rsp.Code, "wrong response status")
			require.Equal(t, test.wantContentType, rsp.Header().Get("Content-Type"), "wrong Content-Type")
			require.Equal(t, test.wantBody, rsp.Body.String(), "wrong response body")
		})
	}
}
