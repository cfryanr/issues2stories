package trackerimport

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"

	"issues2stories/internal/githubapi"
	"issues2stories/internal/importtypes"
)

type handler struct {
	gitHubClient githubapi.GitHubAPI
}

func NewHandler(gitHubClient githubapi.GitHubAPI) http.Handler {
	return &handler{gitHubClient: gitHubClient}
}

// This endpoint implements Tracker's "Import API URL" specification.
// See https://www.pivotaltracker.com/help/articles/other_integration
//
// Note this interesting limitation from the Tracker docs: "There isn’t a numeric limit,
// but the integration will timeout if it takes more than 60 seconds to retrieve all
// results, due to number of items, Internet speed, or the size of the items."
// This code tries to be efficient, but this may theoretically impact GitHub repositories
// which have a very, very large number of open issues. GitHub paginates API results, so
// we need to make an API call to GitHub per 100 issues, adding latency.
func (h *handler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		msg := fmt.Sprintf("Request method is not supported: %s", request.Method)
		log.Print(msg)
		http.Error(responseWriter, msg, http.StatusMethodNotAllowed)
		return
	}

	issues, err := h.gitHubClient.ListAllOpenIssuesForRepoInImportFormat(request.Context())
	if err != nil {
		log.Printf("tracker_import: error getting issues from GitHub API: %v", err)
		http.Error(responseWriter, "failed to get issues from GitHub API", http.StatusBadGateway)
		return
	}

	issuesWithPRsRemoved := make([]importtypes.Issue, 0)
	for _, issue := range issues {
		if issue.PullRequest == nil {
			log.Printf("tracker_import: saw issue #%d: %s", issue.Number, issue.Title)
			issue.RequestedBy = issue.User.Login // promote this field to top-level for xml output
			if issue.HasLabel("bug") {
				issue.StoryType = "bug"
			} else {
				issue.StoryType = "feature"
			}
			issuesWithPRsRemoved = append(issuesWithPRsRemoved, issue)
		}
	}

	xmlIssues := importtypes.IssueList{Issues: issuesWithPRsRemoved}
	xmlIssues.XMLTypeAttr = "array" // Tracker docs say that this element should be annotated with type="array"

	out, err := xml.MarshalIndent(xmlIssues, " ", "  ")
	if err != nil {
		log.Printf("tracker_import: error serializing issues to XML: %v", err)
		http.Error(responseWriter, "error serializing issues to XML", http.StatusInternalServerError)
		return
	}

	responseWriter.Write([]byte(xml.Header))
	responseWriter.Write(out)
}
