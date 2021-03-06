package trackeractivity

import (
	"context"
	"errors"
	"fmt"
	"io"
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

type readerWhichAlwaysErrors int

func (readerWhichAlwaysErrors) Read(_ []byte) (n int, err error) {
	return 0, errors.New("some error")
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := ioutil.ReadFile("testdata/" + name + ".json")
	require.NoError(t, err)
	return string(content)
}

type fakeGitHubGetIssueReturnValues struct {
	issues []*githubapi.Issue
	errors []error
}

type fakeGitHubGetIssueActivity struct {
	invocations     int
	issueNumberArgs []int
}

type fakeGitHubGetIssue struct {
	returns *fakeGitHubGetIssueReturnValues
	actual  *fakeGitHubGetIssueActivity
}

type fakeGitHubUpdateIssueReturnValues struct {
	errors []error
}

type fakeGitHubUpdateIssueActivity struct {
	invocations     int
	issueNumberArgs []int
	updatesArgs     []*github.IssueRequest
}

type fakeGitHubUpdateIssue struct {
	returns *fakeGitHubUpdateIssueReturnValues
	actual  *fakeGitHubUpdateIssueActivity
}

type fakeGitHubAPI struct {
	getIssue    *fakeGitHubGetIssue
	updateIssue *fakeGitHubUpdateIssue
}

func (f *fakeGitHubAPI) GetIssue(_ context.Context, issueNumber int) (*githubapi.Issue, error) {
	thisCall := f.getIssue.actual.invocations
	f.getIssue.actual.invocations++
	f.getIssue.actual.issueNumberArgs = append(f.getIssue.actual.issueNumberArgs, issueNumber)
	if f.getIssue.returns.errors != nil && f.getIssue.returns.errors[thisCall] != nil {
		return nil, f.getIssue.returns.errors[thisCall]
	}
	return f.getIssue.returns.issues[thisCall], nil
}

func (f *fakeGitHubAPI) UpdateIssue(_ context.Context, issueNumber int, updates *github.IssueRequest) error {
	thisCall := f.updateIssue.actual.invocations
	f.updateIssue.actual.invocations++
	f.updateIssue.actual.issueNumberArgs = append(f.updateIssue.actual.issueNumberArgs, issueNumber)
	f.updateIssue.actual.updatesArgs = append(f.updateIssue.actual.updatesArgs, updates)
	if f.updateIssue.returns != nil && f.updateIssue.returns.errors != nil && f.updateIssue.returns.errors[thisCall] != nil {
		return f.updateIssue.returns.errors[thisCall]
	}
	return nil
}

func (f *fakeGitHubAPI) ListAllOpenIssuesForRepoInImportFormat(_ context.Context) ([]importtypes.Issue, error) {
	panic("not used by the test subject")
}

type fakeTrackerAPIReturnValues struct {
	issueIDs []int
	errors   []error
}

type fakeTrackerAPIActivity struct {
	invocations   int
	projectIDArgs []int64
	storyIDArgs   []int64
}

type fakeTrackerAPI struct {
	returns *fakeTrackerAPIReturnValues
	actual  *fakeTrackerAPIActivity
}

func (f *fakeTrackerAPI) GetGithubIssueIDLinkedToStory(trackerProjectID, trackerStoryID int64) (githubIssueID int, err error) {
	thisCall := f.actual.invocations
	f.actual.invocations++
	f.actual.projectIDArgs = append(f.actual.projectIDArgs, trackerProjectID)
	f.actual.storyIDArgs = append(f.actual.storyIDArgs, trackerStoryID)
	if f.returns != nil && f.returns.errors != nil && f.returns.errors[thisCall] != nil {
		return 0, f.returns.errors[thisCall]
	}
	return f.returns.issueIDs[thisCall], nil
}

func TestHandleTrackerActivityWebhook(t *testing.T) {
	tests := []struct {
		name string

		configuration *config.Config

		method      string
		contentType string
		body        string
		bodyFixture string
		bodyReader  io.Reader
		requestAuth *config.BasicAuthCredentials

		wantStatus      int
		wantBody        string
		wantContentType string

		trackerReturns         *fakeTrackerAPIReturnValues
		wantTrackerInvocations *fakeTrackerAPIActivity

		gitHubGetIssueReturns            *fakeGitHubGetIssueReturnValues
		gitHubUpdateIssueReturns         *fakeGitHubUpdateIssueReturnValues
		wantGitHubUpdateIssueInvocations *fakeGitHubUpdateIssueActivity
		wantGitHubGetIssueInvocations    *fakeGitHubGetIssueActivity
	}{
		{
			name:            "wrong method is an error",
			method:          http.MethodGet,
			wantStatus:      http.StatusMethodNotAllowed,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Request method is not supported: GET\n",
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
			requestAuth:     &config.BasicAuthCredentials{Username: "", Password: ""},
			wantStatus:      http.StatusUnauthorized,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Unauthorized\n",
		},
		{
			name:            "wrong content type is an error",
			contentType:     "application/wrong-type",
			wantStatus:      http.StatusUnsupportedMediaType,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Request had wrong Content-Type: application/wrong-type\n",
		},
		{
			name:            "error reading request body",
			bodyReader:      readerWhichAlwaysErrors(0),
			wantStatus:      http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't read body\n",
		},
		{
			name:            "body is not json is an error",
			body:            "this is not valid json",
			wantStatus:      http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't parse json body\n",
		},
		{
			name:        "asking Tracker for the Github issue ID fails",
			bodyFixture: "create_feature_story_in_icebox",
			trackerReturns: &fakeTrackerAPIReturnValues{
				errors: []error{fmt.Errorf("fake error from Tracker")},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantStatus:      http.StatusBadGateway,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't get GitHub issue id from Tracker\n",
		},
		{
			name:        "after asking Tracker for the Github issue ID fails, keep trying the other stories, every story fails",
			bodyFixture: "edit_add_labels_to_multiple_stories",
			trackerReturns: &fakeTrackerAPIReturnValues{
				errors: []error{fmt.Errorf("fake error from Tracker"), fmt.Errorf("fake error from Tracker")},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   2,
				projectIDArgs: []int64{2453999, 2453999},
				storyIDArgs:   []int64{176669667, 176669670},
			},
			wantStatus:      http.StatusBadGateway,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't get GitHub issue id from Tracker\ncan't get GitHub issue id from Tracker\n",
		},
		{
			name:        "creating a Tracker story which is not linked to a github issue does not call github",
			bodyFixture: "create_feature_story_in_icebox",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{0},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "editing a Tracker story's labels when the story is not linked to a github issue does not call github",
			bodyFixture: "edit_add_label_to_story",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{0},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "editing multiple Tracker stories labels when none are linked to github issues does not call github",
			bodyFixture: "edit_add_labels_to_multiple_stories",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{0, 0},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   2,
				projectIDArgs: []int64{2453999, 2453999},
				storyIDArgs:   []int64{176669667, 176669670},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "deleting a story does not call Tracker for the full story details, since deleted stories cannot be queried",
			bodyFixture: "delete_story",
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations: 0,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "creating a feature story in the icebox which is linked to a GitHub issue",
			bodyFixture: "create_feature_story_in_icebox",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "priority/undecided", "enhancement"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "creating a feature story in the backlog which is linked to a GitHub issue",
			bodyFixture: "create_feature_story_in_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176710437},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "priority/backlog", "enhancement"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "creating a bug story in the icebox which is linked to a GitHub issue",
			bodyFixture: "create_bug_story_in_icebox",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176710594},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "priority/undecided", "bug"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "creating a bug story in the backlog which is linked to a GitHub issue",
			bodyFixture: "create_bug_story_in_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176710638},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "priority/backlog", "bug"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "changing a story from feature to bug in the backlog",
			bodyFixture: "edit_change_story_type_feature_to_bug",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "priority/backlog", "bug"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "moving a story from the icebox to the backlog",
			bodyFixture: "move_story_from_icebox_to_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "priority/undecided", "enhancement"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "enhancement", "priority/backlog"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "moving multiple stories from the icebox to the backlog",
			bodyFixture: "move_multiple_stories_from_icebox_to_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42, 43},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{
					{Labels: []string{"initial-unrelated-label1", "priority/undecided", "enhancement"}},
					{Labels: []string{"initial-unrelated-label2", "priority/undecided", "bug"}},
				},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   2,
				projectIDArgs: []int64{2453999, 2453999},
				storyIDArgs:   []int64{176710975, 176710977},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     2,
				issueNumberArgs: []int{42, 43},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     2,
				issueNumberArgs: []int{42, 43},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label1", "enhancement", "priority/backlog"}},
					{Labels: &[]string{"initial-unrelated-label2", "bug", "priority/backlog"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "while editing multiple stories, when the first request to get the GitHub issue details fails, the other issue is still updated",
			bodyFixture: "move_multiple_stories_from_icebox_to_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42, 43},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{
					nil,
					{Labels: []string{"initial-unrelated-label2", "priority/undecided", "bug"}},
				},
				errors: []error{
					fmt.Errorf("fake GitHub API error"),
					nil,
				},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   2,
				projectIDArgs: []int64{2453999, 2453999},
				storyIDArgs:   []int64{176710975, 176710977},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     2,
				issueNumberArgs: []int{42, 43},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{43},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label2", "bug", "priority/backlog"}},
				},
			},
			wantStatus:      http.StatusBadGateway,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't get GitHub issue details from GitHub\n",
		},
		{
			name:        "while editing multiple stories, when the first request to update the GitHub issue fails, the other issue is still updated",
			bodyFixture: "move_multiple_stories_from_icebox_to_backlog",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42, 43},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{
					{Labels: []string{"initial-unrelated-label1", "priority/undecided", "enhancement"}},
					{Labels: []string{"initial-unrelated-label2", "priority/undecided", "bug"}},
				},
			},
			gitHubUpdateIssueReturns: &fakeGitHubUpdateIssueReturnValues{
				errors: []error{
					fmt.Errorf("fake GitHub API error"),
					nil,
				},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   2,
				projectIDArgs: []int64{2453999, 2453999},
				storyIDArgs:   []int64{176710975, 176710977},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     2,
				issueNumberArgs: []int{42, 43},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     2, // also checking the arguments to the first (failed) invocation
				issueNumberArgs: []int{42, 43},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label1", "enhancement", "priority/backlog"}},
					{Labels: &[]string{"initial-unrelated-label2", "bug", "priority/backlog"}},
				},
			},
			wantStatus:      http.StatusBadGateway,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't update GitHub issue via GitHub API\n",
		},
		{
			name:        "estimating a story",
			bodyFixture: "edit_estimate_feature_story",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "enhancement", "priority/backlog", "estimate/XXL"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "skip calling the github API to update the issue when the labels actually didn't change",
			bodyFixture: "edit_estimate_feature_story",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				// "estimate/XXL" is the same label that would be applied in this case, but it was already on the GitHub issue for some reason.
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations: 0,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "removing the estimate from a story",
			bodyFixture: "edit_remove_feature_story_estimate",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{Labels: &[]string{"initial-unrelated-label", "enhancement", "priority/backlog"}},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "changing the owners of a story when all owners are found in configuration",
			bodyFixture: "edit_story_assign_second_owner",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344177: "github-user1",
				3344175: "github-user2",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    nil, // nothing to update
						Assignees: &[]string{"github-user1", "github-user2"},
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:          "changing the owners of a story when no configuration is provided",
			bodyFixture:   "edit_story_assign_second_owner",
			configuration: &config.Config{UserIDMapping: nil},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations: 0, // no labels or owners need updates
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "changing the owners of a story when some of the new owners are not in the configuration map",
			bodyFixture: "edit_story_assign_second_owner",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344175: "github-user2",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    nil, // nothing to update
						Assignees: &[]string{"github-user2"},
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:          "changing the owners of a story when none of the new owners are in the configuration map",
			bodyFixture:   "edit_story_assign_second_owner",
			configuration: &config.Config{UserIDMapping: map[int64]string{}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations: 0, // no labels or owners need updates
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "when the owners have not changed, do not update the issue's assignees",
			bodyFixture: "edit_estimate_feature_story",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344177: "github-user1",
				3344175: "github-user2",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XS", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    &[]string{"initial-unrelated-label", "enhancement", "priority/backlog", "estimate/XXL"},
						Assignees: nil,
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "when all of the owners were explicitly removed, also remove the issue's assignees",
			bodyFixture: "edit_story_remove_all_owners",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344177: "github-user1",
				3344175: "github-user2",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "estimate/XXL", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176711643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    nil,         // nothing to update
						Assignees: &[]string{}, // overwrite assignees with empty list, i.e. remove all assignees
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "do not clear issue assignees when a story is first created",
			bodyFixture: "create_feature_story_in_icebox",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344177: "github-user1",
				3344175: "github-user2",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176650922},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    &[]string{"initial-unrelated-label", "priority/undecided", "enhancement"},
						Assignees: nil, // no update of assignees
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "starting a story from the icebox",
			bodyFixture: "edit_start_story_from_icebox",
			configuration: &config.Config{UserIDMapping: map[int64]string{
				3344177: "github-user1",
			}},
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/undecided", "estimate/XXL"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176755643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels:    &[]string{"initial-unrelated-label", "enhancement", "estimate/XXL", "priority/backlog", "state/started"},
						Assignees: &[]string{"github-user1"},
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "accepting a story labels the issue as accepted, removes the issue's `priority/backlog` label, and also closes the issue",
			bodyFixture: "edit_accept_story",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog", "estimate/XXL", "state/delivered"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176755643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels: &[]string{"initial-unrelated-label", "enhancement", "estimate/XXL", "state/accepted"},
						State:  addressOf("closed"),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "editing an accepted story back to any other state relabels the issue and also reopens the issue",
			bodyFixture: "edit_unaccept_story",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "estimate/XXL", "state/accepted"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176755643},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Labels: &[]string{"initial-unrelated-label", "enhancement", "estimate/XXL", "priority/backlog", "state/started"},
						State:  addressOf("open"),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "editing the title of a story also edits the title of the issue",
			bodyFixture: "edit_story_change_title",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176858613},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Title: addressOf("New title for Fake issue for testing, please ignore"),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "editing the description of a story also edits the title of the issue",
			bodyFixture: "edit_story_change_description",
			trackerReturns: &fakeTrackerAPIReturnValues{
				issueIDs: []int{42},
			},
			gitHubGetIssueReturns: &fakeGitHubGetIssueReturnValues{
				issues: []*githubapi.Issue{{Labels: []string{"initial-unrelated-label", "enhancement", "priority/backlog"}}},
			},
			wantTrackerInvocations: &fakeTrackerAPIActivity{
				invocations:   1,
				projectIDArgs: []int64{2453999},
				storyIDArgs:   []int64{176858613},
			},
			wantGitHubGetIssueInvocations: &fakeGitHubGetIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
			},
			wantGitHubUpdateIssueInvocations: &fakeGitHubUpdateIssueActivity{
				invocations:     1,
				issueNumberArgs: []int{42},
				updatesArgs: []*github.IssueRequest{
					{
						Body: addressOf("This is the UPDATED description.\n"),
					},
				},
			},
			wantStatus: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trackerAPI := fakeTrackerAPI{
				returns: test.trackerReturns,
				actual:  &fakeTrackerAPIActivity{},
			}
			if test.wantTrackerInvocations == nil {
				test.wantTrackerInvocations = &fakeTrackerAPIActivity{}
			}

			gitHubAPI := fakeGitHubAPI{
				getIssue: &fakeGitHubGetIssue{
					returns: test.gitHubGetIssueReturns,
					actual:  &fakeGitHubGetIssueActivity{},
				},
				updateIssue: &fakeGitHubUpdateIssue{
					returns: test.gitHubUpdateIssueReturns,
					actual:  &fakeGitHubUpdateIssueActivity{},
				},
			}
			if test.wantGitHubGetIssueInvocations == nil {
				test.wantGitHubGetIssueInvocations = &fakeGitHubGetIssueActivity{}
			}
			if test.wantGitHubUpdateIssueInvocations == nil {
				test.wantGitHubUpdateIssueInvocations = &fakeGitHubUpdateIssueActivity{}
			}

			if test.method == "" {
				test.method = http.MethodPost
			}
			if test.contentType == "" {
				test.contentType = "application/json"
			}
			if test.configuration == nil {
				test.configuration = &config.Config{}
			}

			subject := NewHandler(&trackerAPI, &gitHubAPI, test.configuration,
				&config.BasicAuthCredentials{Username: "correct-username", Password: "correct-password"})

			var requestBodyReader io.Reader
			switch {
			case test.bodyReader != nil:
				requestBodyReader = test.bodyReader
			case test.bodyFixture != "":
				requestBodyReader = strings.NewReader(readFixture(t, test.bodyFixture))
			default:
				requestBodyReader = strings.NewReader(test.body)
			}

			path := "/some/path?username=correct-username&password=correct-password"
			if test.requestAuth != nil {
				if test.requestAuth.Username == "" && test.requestAuth.Password == "" {
					path = "/some/path"
				} else {
					path = fmt.Sprintf("/some/path?username=%s&password=%s", test.requestAuth.Username, test.requestAuth.Password)
				}
			}

			req := httptest.NewRequest(test.method, path, requestBodyReader)
			rsp := httptest.NewRecorder()
			req.Header.Set("Content-Type", test.contentType)

			subject.ServeHTTP(rsp, req)

			require.Equal(t, test.wantStatus, rsp.Code, "wrong response status")
			require.Equal(t, test.wantContentType, rsp.Header().Get("Content-Type"), "wrong Content-Type")
			require.Equal(t, test.wantBody, rsp.Body.String(), "wrong response body")

			require.Equal(t, test.wantTrackerInvocations.invocations, trackerAPI.actual.invocations, "wrong number of Tracker API invocations")
			require.Equal(t, test.wantTrackerInvocations.projectIDArgs, trackerAPI.actual.projectIDArgs, "wrong Tracker project ID arguments")
			require.Equal(t, test.wantTrackerInvocations.storyIDArgs, trackerAPI.actual.storyIDArgs, "wrong Tracker story ID arguments")

			require.Equal(t, test.wantGitHubGetIssueInvocations.invocations, gitHubAPI.getIssue.actual.invocations, "wrong number of GitHub GetIssue() API invocations")
			require.Equal(t, test.wantGitHubGetIssueInvocations.issueNumberArgs, gitHubAPI.getIssue.actual.issueNumberArgs, "wrong GitHub GetIssue() issue arguments")

			require.Equal(t, test.wantGitHubUpdateIssueInvocations.invocations, gitHubAPI.updateIssue.actual.invocations, "wrong number of GitHub UpdateIssue() API invocations")
			require.Equal(t, test.wantGitHubUpdateIssueInvocations.issueNumberArgs, gitHubAPI.updateIssue.actual.issueNumberArgs, "wrong GitHub UpdateIssue() issue arguments")
			require.Equal(t, test.wantGitHubUpdateIssueInvocations.updatesArgs, gitHubAPI.updateIssue.actual.updatesArgs, "wrong GitHub UpdateIssue() updates arguments")
		})
	}
}
