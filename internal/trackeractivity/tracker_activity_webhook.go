package trackeractivity

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

type TrackerAPI interface {
	GetGithubIssueIDLinkedToStory(trackerProjectID, trackerStoryID int64) (githubIssueID int, err error)
}

type TrackerEvent struct {
	Kind    string   `json:"kind"`
	Changes []Change `json:"changes"`
	Project Project  `json:"project"`
}

type Change struct {
	Kind       string `json:"kind"`
	ID         int64  `json:"id"`
	ChangeType string `json:"change_type"`
	StoryType  string `json:"story_type"`
}

type Project struct {
	ID int64 `json:"id"`
}

type handler struct {
	TrackerAPI TrackerAPI
}

func NewHandler(trackerAPI TrackerAPI) http.Handler {
	return &handler{TrackerAPI: trackerAPI}
}

// This endpoint implements Tracker's "Activity Web Hook" specification.
// See https://www.pivotaltracker.com/help/articles/activity_webhook
func (h *handler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != "POST" {
		msg := fmt.Sprintf("Request method is not supported: %s", request.Method)
		log.Print(msg)
		http.Error(responseWriter, msg, http.StatusMethodNotAllowed)
		return
	}

	contentType := request.Header.Get("Content-Type")
	if contentType != "application/json" {
		msg := fmt.Sprintf("Request had wrong Content-Type: %s", contentType)
		log.Print(msg)
		http.Error(responseWriter, msg, http.StatusUnsupportedMediaType)
		return
	}

	log.Printf("Got POST request: %+v", request)
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		http.Error(responseWriter, "can't read body", http.StatusBadRequest)
		return
	}
	log.Printf("Request body: %+v", string(body))

	var activityEvent TrackerEvent
	err = json.Unmarshal(body, &activityEvent)
	if err != nil {
		log.Printf("Error parsing request body: %v", err)
		http.Error(responseWriter, "can't parse json body", http.StatusBadRequest)
		return
	}

	log.Printf("Saw event: kind %s, project %d", activityEvent.Kind, activityEvent.Project.ID)

	for _, change := range activityEvent.Changes {
		if change.Kind != "story" {
			continue
		}

		// TODO only watch specific story change types that we care about

		log.Printf("Saw story change: kind %s, story %d, story_type %s", change.ChangeType, change.ID, change.StoryType)

		githubIssueID, err := h.TrackerAPI.GetGithubIssueIDLinkedToStory(activityEvent.Project.ID, change.ID)
		if err != nil {
			// TODO when a story is being deleted, trying to query it results in a 404... do we care??
			log.Printf("Error calling Tracker API: %v", err)
			http.Error(responseWriter, "can't get github issue id", http.StatusBadGateway)
			return
		}

		if githubIssueID == 0 {
			// this Tracker story is not linked to a GitHub Issue, so skip it
			log.Printf("Story is not linked to GitHub issue: story %d", change.ID)
			continue
		}

		log.Printf("Story is linked to GitHub issue: story %d, GitHub issuse %d", change.ID, githubIssueID)
		// TODO call github and do something with the issue id when it is non-zero
	}
}
