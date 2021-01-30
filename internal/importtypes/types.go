package importtypes

import (
	"encoding/xml"
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

func (i *Issue) HasLabel(labelName string) bool {
	for _, label := range i.Labels {
		if label.Name == labelName {
			return true
		}
	}
	return false
}
