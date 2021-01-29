package trackeractivity

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/google/go-github/v33/github"
	"issues2stories/internal/githubapi"
	"issues2stories/internal/trackerapi"
)

type handler struct {
	trackerAPI   trackerapi.TrackerAPI
	gitHubClient githubapi.GitHubAPI

	labelsToRemoveOnStateChange []string
	labelsToRemoveOnTypeChange  []string
}

func NewHandler(trackerAPI trackerapi.TrackerAPI, gitHubClient githubapi.GitHubAPI) http.Handler {
	return &handler{
		trackerAPI:   trackerAPI,
		gitHubClient: gitHubClient,

		labelsToRemoveOnStateChange: uniqueValuesFromMapOfSlices(issueLabelsToApplyPerStoryState),
		labelsToRemoveOnTypeChange:  uniqueValuesFromMapOfSlices(issueLabelsToApplyPerStoryType),
	}
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

		log.Printf("Saw story change: kind %s, story %d, story_type %s", change.ChangeType, change.ID, change.StoryType)

		if change.ChangeType == "delete" {
			// A story that is already deleted cannot be queried via the Tracker API,
			// so we have no easy way of knowing if it was linked to a GihHub issue.
			log.Printf("Story was deleted, so skipping: story %d", change.ID)
			continue
		}

		githubIssueID, err := h.trackerAPI.GetGithubIssueIDLinkedToStory(activityEvent.Project.ID, change.ID)
		if err != nil {
			log.Printf("Error calling Tracker API: %v", err)
			http.Error(responseWriter, "can't get GitHub issue id from Tracker", http.StatusBadGateway)
			continue
		}

		if githubIssueID == 0 {
			// This Tracker story is not linked to a GitHub Issue, so skip it.
			log.Printf("Story is not linked to GitHub issue: story %d", change.ID)
			continue
		}

		log.Printf("Story is linked to GitHub issue: story %d, GitHub issuse %d", change.ID, githubIssueID)

		issueDetails, err := h.gitHubClient.GetIssue(request.Context(), githubIssueID)
		if err != nil {
			log.Printf("Could not get issue #%d from github: %v", githubIssueID, err)
			http.Error(responseWriter, "can't get GitHub issue details from GitHub", http.StatusBadGateway)
			continue
		}

		issueLabels := issueDetails.Labels
		log.Printf("issue #%d had labels before update: %v", githubIssueID, issueLabels)

		// If the current state of the story has changed, then update the labels of the linked issue.
		newStoryState := change.NewValues.CurrentState
		if newStoryState != "" {
			issueLabels = removeElements(issueLabels, h.labelsToRemoveOnStateChange)
			labelsForNewState := issueLabelsToApplyPerStoryState[newStoryState]
			issueLabels = append(issueLabels, labelsForNewState...)
		}

		// If the story type has changed, then update the labels of the linked issue.
		newStoryType := change.NewValues.StoryType
		if newStoryType != "" {
			issueLabels = removeElements(issueLabels, h.labelsToRemoveOnTypeChange)
			labelsForNewStoryType := issueLabelsToApplyPerStoryType[newStoryType]
			issueLabels = append(issueLabels, labelsForNewStoryType...)
		}

		// Push the updates back to GitHub.
		err = h.gitHubClient.UpdateIssue(request.Context(), githubIssueID, &github.IssueRequest{Labels: &issueLabels})
		if err != nil {
			log.Printf("Error calling GitHub API: %v", err)
			http.Error(responseWriter, "can't update GitHub issue via GitHub API", http.StatusBadGateway)
			continue
		}
	}
}
