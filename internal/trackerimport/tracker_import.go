package trackerimport

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
)

type IssueList struct {
	XMLName     xml.Name `xml:"external_stories"`
	XMLTypeAttr string   `xml:"type,attr"`
	Issues      []Issue
}

type User struct {
	Login string
}

type PullRequest struct {
	URL string
}

type Label struct {
	Name string
}

type Issue struct {
	XMLName     xml.Name     `xml:"external_story"`
	HtmlUrl     string       `json:"html_url" xml:",comment"`
	PullRequest *PullRequest `json:"pull_request" xml:"-"`
	Number      int          `xml:"external_id"`
	Title       string       `xml:"name"`
	Body        string       `xml:"description"`
	User        User         `xml:"-"`
	RequestedBy string       `xml:"requested_by"`
	StoryType   string       `xml:"story_type"`
	Labels      []Label      `xml:"-"`
	CreatedAt   time.Time    `json:"created_at,string" xml:"created_at"`
}

// This endpoint implements Tracker's "Import API URL" specification.
// See https://www.pivotaltracker.com/help/articles/other_integration
func HandleTrackerImport(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != "GET" {
		msg := "Method is not supported"
		log.Printf("%v", msg)
		http.Error(responseWriter, msg, http.StatusMethodNotAllowed)
		return
	}

	httpClient := &http.Client{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	getIssuesRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf(
			"https://api.github.com/repos/%s/%s/issues?state=open",
			os.Getenv("GITHUB_ORG"),
			os.Getenv("GITHUB_REPO"),
		),
		nil,
	)

	gitHubResp, err := httpClient.Do(getIssuesRequest)
	if err != nil {
		log.Printf("%v", err)
		http.Error(responseWriter, "Failed to get Issues from GitHub API.", http.StatusInternalServerError)
		return
	}
	defer gitHubResp.Body.Close()

	if gitHubResp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("GitHub API returned non-OK status: %s", gitHubResp.Status)
		log.Printf("%v", msg)
		http.Error(responseWriter, msg, http.StatusInternalServerError)
		return
	}

	bodyBytes, err := ioutil.ReadAll(gitHubResp.Body)
	if err != nil {
		log.Printf("%v", err)
		http.Error(responseWriter, "Failed to read GitHub response body", http.StatusInternalServerError)
		return
	}

	var issues []Issue
	err = json.Unmarshal(bodyBytes, &issues)
	if err != nil {
		log.Printf("%v", err)
		http.Error(responseWriter, "Failed to parse Issues response from GitHub API.", http.StatusInternalServerError)
		return
	}

	issuesWithPRsRemoved := make([]Issue, 0)
	for _, issue := range issues {
		if issue.PullRequest == nil {
			issue.RequestedBy = issue.User.Login // promote this field to top-level for xml output
			if issue.HasLabel("bug") {
				issue.StoryType = "bug"
			} else {
				issue.StoryType = "feature"
			}
			issuesWithPRsRemoved = append(issuesWithPRsRemoved, issue)
		}
	}

	xmlIssues := IssueList{Issues: issuesWithPRsRemoved}
	xmlIssues.XMLTypeAttr = "array" // Tracker docs say that this element should be annotated with type="array"

	out, err := xml.MarshalIndent(xmlIssues, " ", "  ")
	if err != nil {
		msg := "Failed to serialize issues to XML"
		log.Printf("%v", msg)
		http.Error(responseWriter, msg, http.StatusInternalServerError)
		return
	}

	responseWriter.Write([]byte(xml.Header))
	responseWriter.Write(out)
}

func (i *Issue) HasLabel(labelName string) bool {
	for _, label := range i.Labels {
		if label.Name == labelName {
			return true
		}
	}
	return false
}
