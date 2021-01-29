package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
	"issues2stories/internal/config"
	"issues2stories/internal/githubapi"
	"issues2stories/internal/trackeractivity"
	"issues2stories/internal/trackerapi"
	"issues2stories/internal/trackerimport"
)

// Respond to heath checks at the root path. GKE Ingress default health check is GET "/".
func defaultHandler(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		log.Printf("defaultHandler: path not found: %s", request.URL.Path)
		http.Error(responseWriter, "404. Not found.", http.StatusNotFound)
		return
	}

	if request.Method != "GET" {
		msg := "Method is not supported"
		log.Printf("defaultHandler: %v", msg)
		http.Error(responseWriter, msg, http.StatusMethodNotAllowed)
		return
	}

	log.Print("defaultHandler: responding ok to GET /")
	responseWriter.Write([]byte("ok"))
}

func main() {
	fmt.Print("Starting server at port 8080\n")

	const configFilePath = "/etc/config/config.yaml"
	configYAML, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("could not read config file: %s", configFilePath)
	}
	configuration := config.Config{}
	err = yaml.Unmarshal(configYAML, &configuration)
	if err != nil {
		log.Fatalf("could not parse config file (%s) as YAML: %v", configFilePath, err)
	}
	log.Printf("Read user ID mapping config: %v", configuration.UserIDMapping)

	gitHubOrg, ok := os.LookupEnv("GITHUB_ORG")
	if !ok || gitHubOrg == "" {
		log.Fatal("GITHUB_ORG environment variable not found or empty")
	}
	gitHubRepo, ok := os.LookupEnv("GITHUB_REPO")
	if !ok || gitHubRepo == "" {
		log.Fatal("GITHUB_REPO environment variable not found or empty")
	}
	trackerAPIToken, ok := os.LookupEnv("TRACKER_API_TOKEN")
	if !ok || trackerAPIToken == "" {
		log.Fatal("TRACKER_API_TOKEN environment variable not found or empty")
	}

	trackerClient := trackerapi.New(trackerAPIToken, &http.Client{})
	gitHubClient := githubapi.New()

	mux := http.NewServeMux()
	mux.Handle("/tracker_activity", trackeractivity.NewHandler(trackerClient, gitHubClient, &configuration))
	mux.Handle("/tracker_import", http.HandlerFunc(trackerimport.HandleTrackerImport))
	mux.Handle("/", http.HandlerFunc(defaultHandler))

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
