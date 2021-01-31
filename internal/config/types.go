package config

import (
	"net/http"
)

type Config struct {
	// Note that UserIDMapping can be nil.
	UserIDMapping map[int64]string `yaml:"tracker_id_to_github_username_mapping"`
}

type BasicAuthCredentials struct {
	Username, Password string
}

func (b *BasicAuthCredentials) Matches(request *http.Request) bool {
	// Try getting the credentials from the Authorization header.
	basicAuthUser, basicAuthPass, ok := request.BasicAuth()
	if ok {
		return b.Username == basicAuthUser && b.Password == basicAuthPass
	}

	// Otherwise try getting the credentials from query parameters.
	// We do this because Tracker webhooks don't send basic auth
	// headers, but do allow configuring arbitrary query parameters.
	queryUser := request.URL.Query()["username"]
	queryPass := request.URL.Query()["password"]
	if len(queryUser) > 0 && len(queryPass) > 0 {
		return b.Username == queryUser[0] && b.Password == queryPass[0]
	}

	// Otherwise authentication failed.
	return false
}
