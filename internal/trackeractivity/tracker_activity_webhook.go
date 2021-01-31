package trackeractivity

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/google/go-github/v33/github"
	"issues2stories/internal/config"
	"issues2stories/internal/githubapi"
	"issues2stories/internal/trackerapi"
)

type handler struct {
	trackerAPI   trackerapi.TrackerAPI
	gitHubClient githubapi.GitHubAPI

	configuration *config.Config
	credentials   *config.BasicAuthCredentials

	labelsToRemoveOnStateChange    []string
	labelsToRemoveOnTypeChange     []string
	labelsToRemoveOnEstimateChange []string
}

func NewHandler(trackerAPI trackerapi.TrackerAPI, gitHubClient githubapi.GitHubAPI, configuration *config.Config, credentials *config.BasicAuthCredentials) http.Handler {
	return &handler{
		trackerAPI:    trackerAPI,
		gitHubClient:  gitHubClient,
		configuration: configuration,
		credentials:   credentials,

		labelsToRemoveOnStateChange:    uniqueValuesFromMapOfSlices(issueLabelsToApplyPerStoryState),
		labelsToRemoveOnTypeChange:     uniqueValuesFromMapOfSlices(issueLabelsToApplyPerStoryType),
		labelsToRemoveOnEstimateChange: uniqueValuesFromMapOfSlices(issueLabelsToApplyPerStoryEstimate),
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

	if !h.credentials.Matches(request) {
		log.Print("Rejecting request due to bad credentials.")
		http.Error(responseWriter, "Unauthorized", http.StatusUnauthorized)
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

		// Get the GitHub issue's initial list of labels.
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

		// If the story's estimate has changed, then update the labels of the linked issue.
		if change.NewValues.Estimate.Present {
			issueLabels = removeElements(issueLabels, h.labelsToRemoveOnEstimateChange)
			// If the new value is nil, then the story was unestimated.
			newEstimate := change.NewValues.Estimate.Value
			if newEstimate != nil {
				// The new value exists, so the story was estimated or re-estimated.
				labelsForNewStoryType := issueLabelsToApplyPerStoryEstimate[fmt.Sprint(*newEstimate)]
				issueLabels = append(issueLabels, labelsForNewStoryType...)
			}
		}

		// All label processing is finished, so set the results on the request object.
		issueRequest := github.IssueRequest{Labels: &issueLabels}
		log.Printf("New labels for issue #%d: %v", githubIssueID, issueLabels)
		if equalIgnoringOrder(issueDetails.Labels, issueLabels) {
			log.Printf("No label updates needed for issue #%d", githubIssueID)
			issueRequest.Labels = nil
		}

		// If the story's owners have changed, then consider overwriting the assignees of the linked issue.
		// Skip this when a story is initially created, because it will always set the owners to empty list
		// in the change object, so there's no point in overwriting the current issue assignees just because
		// the issue was dragged and dropped into the backlog/icebox.
		if change.NewValues.OwnerIDs.Present && h.configuration.UserIDMapping != nil && change.ChangeType != "create" {
			newStoryOwners := *change.NewValues.OwnerIDs.Value
			if len(newStoryOwners) == 0 {
				// All of the previous owners were explicitly removed. Clear the issue assignees list on the issue.
				log.Printf("Previous story owners we explicitly removed. Clearing all assignees on issue #%d", githubIssueID)
				issueRequest.Assignees = &[]string{}
			} else {
				// There are new owners explicitly assigned. Try to find their GitHub usernames.
				newIssueAssignees := []string{}
				for _, ownerID := range newStoryOwners {
					gitHubUsernameOfOwner := h.configuration.UserIDMapping[ownerID]
					if gitHubUsernameOfOwner != "" {
						newIssueAssignees = append(newIssueAssignees, gitHubUsernameOfOwner)
					}
				}
				// If none of the new owners had GitHub usernames configured, then skip the update.
				if len(newIssueAssignees) > 0 {
					log.Printf("Updating issue assignees on issue #%d to: %v", githubIssueID, newIssueAssignees)
					issueRequest.Assignees = &newIssueAssignees
				} else {
					log.Printf(
						"Skipping updating issue #%d assignees: none of these new story owners had GitHub usernames configured: %v",
						githubIssueID, newStoryOwners)
				}
			}
		}

		// Push the updates back to GitHub, if there are any changes to be made.
		if issueRequest.Assignees == nil && issueRequest.Labels == nil {
			log.Printf("No updates planned. Skipping GitHub API call for issue #%d", githubIssueID)
			continue
		}
		log.Printf("Calling GitHub API to update issue #%d", githubIssueID)
		err = h.gitHubClient.UpdateIssue(request.Context(), githubIssueID, &issueRequest)
		if err != nil {
			log.Printf("Error calling GitHub API: %v", err)
			http.Error(responseWriter, "can't update GitHub issue via GitHub API", http.StatusBadGateway)
			continue
		}
	}
}
