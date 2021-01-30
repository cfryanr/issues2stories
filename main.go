package main

import (
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"issues2stories/internal/config"
	"issues2stories/internal/githubapi"
	"issues2stories/internal/trackeractivity"
	"issues2stories/internal/trackerapi"
	"issues2stories/internal/trackerimport"
)

func main() {
	log.Println("Starting server at port 8080")

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

	gitHubOrg := requireEnv("GITHUB_ORG")
	gitHubRepo := requireEnv("GITHUB_REPO")
	gitAPIToken := requireEnv("GITHUB_API_TOKEN")
	trackerAPIToken := requireEnv("TRACKER_API_TOKEN")

	trackerClient := trackerapi.New(trackerAPIToken, &http.Client{})
	gitHubClient := githubapi.New(gitAPIToken, gitHubOrg, gitHubRepo)

	mux := http.NewServeMux()
	mux.Handle("/tracker_activity", trackeractivity.NewHandler(trackerClient, gitHubClient, &configuration))
	mux.Handle("/tracker_import", trackerimport.NewHandler(gitHubClient))
	mux.Handle("/", http.HandlerFunc(defaultHandler))

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func requireEnv(envVarName string) string {
	value, ok := os.LookupEnv(envVarName)
	if !ok || value == "" {
		log.Fatalf("environment variable not found or empty: %s", envVarName)
	}
	return value
}

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
