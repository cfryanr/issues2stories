package trackerapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
)

type TrackerResponse struct {
	ExternalID string `json:"external_id"`
}

type Client struct {
	client          *http.Client
	trackerAPIToken string
}

func New(trackerAPIToken string, client *http.Client) *Client {
	return &Client{trackerAPIToken: trackerAPIToken, client: client}
}

func (c *Client) GetGithubIssueIDLinkedToStory(trackerProjectID, trackerStoryID int64) (githubIssueID int, err error) {
	url := fmt.Sprintf("https://www.pivotaltracker.com/services/v5/projects/%d/stories/%d", trackerProjectID, trackerStoryID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("X-TrackerToken", c.trackerAPIToken)
	res, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("Tracker API request failed: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Tracker API at %s returned status %d", url, res.StatusCode)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("Tracker API at %s returned body which cannot be read: %v", url, err)
	}

	var parsedResponse TrackerResponse
	err = json.Unmarshal(body, &parsedResponse)
	if err != nil {
		return 0, fmt.Errorf("Tracker API at %s returned body which cannot be parsed as json: %s", url, body)
	}

	if parsedResponse.ExternalID != "" {
		parsedInt, err := strconv.Atoi(parsedResponse.ExternalID)
		if err != nil {
			return 0, fmt.Errorf("Tracker API at %s returned non-integer external_id: %s", url, parsedResponse.ExternalID)
		}
		return parsedInt, nil
	}

	return 0, nil
}
