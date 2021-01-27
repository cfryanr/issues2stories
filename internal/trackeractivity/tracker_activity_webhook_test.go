package trackeractivity

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type errReader int

func (errReader) Read(_ []byte) (n int, err error) {
	return 0, errors.New("some error")
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := ioutil.ReadFile("testdata/" + name + ".json")
	require.NoError(t, err)
	return string(content)
}

type fakeTrackerAPI struct {
	callCount            int
	calledWithProjectIDs []int64
	calledWithStoryIDs   []int64
	returnIssueIDs       []int
	returnError          error
}

func (f *fakeTrackerAPI) GetGithubIssueIDLinkedToStory(trackerProjectID, trackerStoryID int64) (githubIssueID int, err error) {
	f.callCount++
	f.calledWithProjectIDs = append(f.calledWithProjectIDs, trackerProjectID)
	f.calledWithStoryIDs = append(f.calledWithStoryIDs, trackerStoryID)
	if f.returnError != nil {
		return 0, f.returnError
	}
	return f.returnIssueIDs[f.callCount-1], nil
}

func TestHandleTrackerActivityWebhook(t *testing.T) {
	tests := []struct {
		name string

		method      string
		path        string
		contentType string
		body        string
		bodyFixture string
		bodyReader  io.Reader

		wantStatus      int
		wantBody        string
		wantContentType string

		wantTrackerCalls       int
		trackerReturnsIssueIDs []int
		wantTrackerProjectIDs  []int64
		wantTrackerStoryIDs    []int64
		trackerReturnsError    error
	}{
		{
			name:            "wrong method is an error",
			method:          http.MethodGet,
			contentType:     "application/json",
			wantStatus:      http.StatusMethodNotAllowed,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Request method is not supported: GET\n",
		},
		{
			name:            "wrong content type is an error",
			method:          http.MethodPost,
			contentType:     "application/wrong-type",
			wantStatus:      http.StatusUnsupportedMediaType,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "Request had wrong Content-Type: application/wrong-type\n",
		},
		{
			name:            "error reading request body",
			method:          http.MethodPost,
			contentType:     "application/json",
			bodyReader:      errReader(0),
			wantStatus:      http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't read body\n",
		},
		{
			name:            "body is not json is an error",
			method:          http.MethodPost,
			contentType:     "application/json",
			body:            "this is not valid json",
			wantStatus:      http.StatusBadRequest,
			wantContentType: "text/plain; charset=utf-8",
			wantBody:        "can't parse json body\n",
		},
		{
			name:                  "asking Tracker for the Github issue ID fails",
			method:                http.MethodPost,
			contentType:           "application/json",
			bodyFixture:           "create_feature_story_in_icebox",
			trackerReturnsError:   fmt.Errorf("fake error from Tracker"),
			wantTrackerCalls:      1,
			wantTrackerProjectIDs: []int64{2453999},
			wantTrackerStoryIDs:   []int64{176650922},
			wantStatus:            http.StatusBadGateway,
			wantContentType:       "text/plain; charset=utf-8",
			wantBody:              "can't get github issue id\n",
		},
		{
			name:                   "creating a Tracker story which is not linked to a github issue does not call github",
			method:                 http.MethodPost,
			contentType:            "application/json",
			bodyFixture:            "create_feature_story_in_icebox",
			trackerReturnsIssueIDs: []int{0},
			wantTrackerCalls:       1,
			wantTrackerProjectIDs:  []int64{2453999},
			wantTrackerStoryIDs:    []int64{176650922},
			wantStatus:             http.StatusOK,
		},
		{
			name:                   "editing a Tracker story's labels when the story is not linked to a github issue does not call github",
			method:                 http.MethodPost,
			contentType:            "application/json",
			bodyFixture:            "edit_add_label_to_story",
			trackerReturnsIssueIDs: []int{0},
			wantTrackerCalls:       1,
			wantTrackerProjectIDs:  []int64{2453999},
			wantTrackerStoryIDs:    []int64{176650922},
			wantStatus:             http.StatusOK,
		},
		{
			name:                   "editing multiple Tracker stories labels when none are linked to github issues does not call github",
			method:                 http.MethodPost,
			contentType:            "application/json",
			bodyFixture:            "edit_add_labels_to_multiple_stories",
			trackerReturnsIssueIDs: []int{0, 0},
			wantTrackerCalls:       2,
			wantTrackerProjectIDs:  []int64{2453999, 2453999},
			wantTrackerStoryIDs:    []int64{176669667, 176669670},
			wantStatus:             http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeTrackerAPI := fakeTrackerAPI{
				returnError:    test.trackerReturnsError,
				returnIssueIDs: test.trackerReturnsIssueIDs,
			}
			subject := NewHandler(&fakeTrackerAPI)

			var requestBodyReader io.Reader
			switch {
			case test.bodyReader != nil:
				requestBodyReader = test.bodyReader
			case test.bodyFixture != "":
				requestBodyReader = strings.NewReader(readFixture(t, test.bodyFixture))
			default:
				requestBodyReader = strings.NewReader(test.body)
			}

			req := httptest.NewRequest(test.method, "/some/path", requestBodyReader)
			rsp := httptest.NewRecorder()
			req.Header.Set("Content-Type", test.contentType)

			subject.ServeHTTP(rsp, req)

			require.Equal(t, test.wantStatus, rsp.Code)
			require.Equal(t, test.wantContentType, rsp.Header().Get("Content-Type"))
			require.Equal(t, test.wantBody, rsp.Body.String())

			require.Equal(t, test.wantTrackerCalls, fakeTrackerAPI.callCount)
			require.Equal(t, test.wantTrackerProjectIDs, fakeTrackerAPI.calledWithProjectIDs)
			require.Equal(t, test.wantTrackerStoryIDs, fakeTrackerAPI.calledWithStoryIDs)
		})
	}
}
