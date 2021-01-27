package trackerapi

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"testing"
)

type errReader int

func (errReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("some error")
}

type RoundTripFunc func(req *http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func NewTestClient(f RoundTripFunc) *http.Client {
	return &http.Client{Transport: f}
}

func TestTrackerAPIClient(t *testing.T) {
	tests := []struct {
		name string

		trackerProjectID int64
		trackerStoryID   int64

		trackerRequestError          error
		trackerResponseBody          string
		trackerResponseBodyReadFails bool
		trackerResponseStatus        int

		wantIssueID int
		wantError   error
	}{
		// Example of request/response for a story which is linked to a GitHub issue. It has an "external_id".
		// ❯ curl -X GET -H "X-TrackerToken: $TRACKER_TOKEN" "https://www.pivotaltracker.com/services/v5/projects/2453999/stories/176651069"
		//{
		//  "kind": "story",
		//  "id": 176651069,
		//  "created_at": "2021-01-26T01:23:12Z",
		//  "updated_at": "2021-01-26T01:25:35Z",
		//  "story_type": "feature",
		//  "name": "Enable audit logging for all of our test environments",
		//  "description": "Example description markdown line 1\nLine 2\n",
		//  "current_state": "unscheduled",
		//  "requested_by_id": 3344177,
		//  "external_id": "348",
		//  "integration_id": 52033,
		//  "url": "https://www.pivotaltracker.com/story/show/176651069",
		//  "project_id": 2453999,
		//  "owner_ids": [],
		//  "labels": []
		//}
		{
			name:                  "returns the external ID for a tracker story which is linked to an issue",
			trackerProjectID:      12345,
			trackerStoryID:        54321,
			trackerResponseBody:   `{"kind": "story", "id": 54321, "external_id": "348"}`,
			trackerResponseStatus: 200,
			wantIssueID:           348,
		},

		// Example of request/response for a story which is NOT linked to a GitHub issue. It has no "external_id".
		//❯ curl -X GET -H "X-TrackerToken: $TRACKER_TOKEN" "https://www.pivotaltracker.com/services/v5/projects/2453999/stories/176208994"
		//{
		//  "kind": "story",
		//  "id": 176208994,
		//  "created_at": "2020-12-17T16:25:11Z",
		//  "updated_at": "2021-01-13T15:54:15Z",
		//  "story_type": "chore",
		//  "name": "Extend our integration test helper to assert that pods do not restart",
		//  "current_state": "started",
		//  "requested_by_id": 3344164,
		//  "url": "https://www.pivotaltracker.com/story/show/176208994",
		//  "project_id": 2453999,
		//  "owner_ids": [
		//    3344175
		//  ],
		//  "labels": [],
		//  "owned_by_id": 3344175
		//}
		{
			name:                  "returns zero for a Tracker story which is not linked to any GitHub issue",
			trackerProjectID:      12345,
			trackerStoryID:        54321,
			trackerResponseBody:   `{"kind": "story", "id": 54321}`,
			trackerResponseStatus: 200,
			wantIssueID:           0,
		},

		{
			name:                  "returns error for any non-200 status code from Tracker",
			trackerProjectID:      12345,
			trackerStoryID:        54321,
			trackerResponseBody:   `{"kind": "error"}`,
			trackerResponseStatus: 404,
			wantError:             fmt.Errorf("Tracker API at https://www.pivotaltracker.com/services/v5/projects/12345/stories/54321 returned status 404"),
		},
		{
			name:                  "returns error for un-parsable GitHub issue ID",
			trackerProjectID:      12345,
			trackerStoryID:        54321,
			trackerResponseBody:   `{"kind": "story", "id": 54321, "external_id": "this is not an int"}`,
			trackerResponseStatus: 200,
			wantError:             fmt.Errorf("Tracker API at https://www.pivotaltracker.com/services/v5/projects/12345/stories/54321 returned non-integer external_id: this is not an int"),
		},
		{
			name:                "returns error when the the http call to Tracker fails",
			trackerProjectID:    12345,
			trackerStoryID:      54321,
			trackerRequestError: fmt.Errorf("some http error"),
			wantError:           fmt.Errorf("Tracker API request failed: Get \"https://www.pivotaltracker.com/services/v5/projects/12345/stories/54321\": some http error"),
		},
		{
			name:                  "returns error when Tracker response cannot be parsed",
			trackerProjectID:      12345,
			trackerStoryID:        54321,
			trackerResponseBody:   "this is not json",
			trackerResponseStatus: 200,
			wantError:             fmt.Errorf("Tracker API at https://www.pivotaltracker.com/services/v5/projects/12345/stories/54321 returned body which cannot be parsed as json: this is not json"),
		},
		{
			name:                         "returns error when Tracker response body cannot be read",
			trackerProjectID:             12345,
			trackerStoryID:               54321,
			trackerResponseBodyReadFails: true,
			trackerResponseStatus:        200,
			wantError:                    fmt.Errorf("Tracker API at https://www.pivotaltracker.com/services/v5/projects/12345/stories/54321 returned body which cannot be read: some error"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			trackerAPIToken := "fake-token"
			clientMadeRequest := false
			client := NewTestClient(func(req *http.Request) (*http.Response, error) {
				clientMadeRequest = true
				expectedURL := fmt.Sprintf(
					"https://www.pivotaltracker.com/services/v5/projects/%d/stories/%d",
					test.trackerProjectID,
					test.trackerStoryID,
				)
				require.Equal(t, "GET", req.Method)
				require.Equal(t, expectedURL, req.URL.String())
				require.Equal(t, trackerAPIToken, req.Header.Get("X-TrackerToken"))

				if test.trackerRequestError != nil {
					return nil, test.trackerRequestError
				}

				body := ioutil.NopCloser(bytes.NewBufferString(test.trackerResponseBody))
				if test.trackerResponseBodyReadFails {
					body = ioutil.NopCloser(errReader(0))
				}
				return &http.Response{
					StatusCode: test.trackerResponseStatus,
					Body:       body,
					Header:     make(http.Header),
				}, nil
			})

			subject := New(trackerAPIToken, client)
			issueID, err := subject.GetGithubIssueIDLinkedToStory(test.trackerProjectID, test.trackerStoryID)

			require.True(t, clientMadeRequest)
			require.Equal(t, test.wantError, err)
			require.Equal(t, test.wantIssueID, issueID)
		})
	}
}
