package main

import (
	"fmt"
	"issues2stories/internal/trackeractivity"
	"issues2stories/internal/trackerapi"
	"issues2stories/internal/trackerimport"
	"log"
	"net/http"
	"os"
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

	mux := http.NewServeMux()
	mux.Handle("/tracker_activity", trackeractivity.NewHandler(trackerapi.New(trackerAPIToken, &http.Client{})))
	mux.Handle("/tracker_import", http.HandlerFunc(trackerimport.HandleTrackerImport))
	mux.Handle("/", http.HandlerFunc(defaultHandler))

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
