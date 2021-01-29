# issues2stories

issues2stories is an app which provides a limited amount of synchronizing between
[Pivotal Tracker](https://www.pivotaltracker.com/) user stories in a Tracker project
and [GitHub](https://github.com/) issues in a GitHub repository.

## Features

issues2stories provides a
[Pivotal Tracker integration](https://www.pivotaltracker.com/help/articles/other_integration)
which adds a new panel to your Tracker project. 
The new panel shows a list of all open GitHub issues from the GitHub repository (not including pull requests).
It can be refreshed using a button at the top of the panel.

When an issue is dragged and dropped from that panel into your icebox
or backlog, then it is automatically converted to a Tracker user story.
Upon creation, the issue description is copied from the issue to the user story.
If the issue is labeled as a bug, then the user story will be created as a bug story.
Otherwise, it will be created as a feature story.

The user story will contain a new field called "ISSUES2STORIES ID",
shown just below where the user story "owners" field is shown.
This field contains a convenient hyperlink to open the linked GitHub issue
in your browser.

The user story can then be edited as usual.
Additional changes to the GitHub issue are *not* reflected in the Tracker user story.
issues2stories also provides a
[Pivotal Tracker webhook](https://www.pivotaltracker.com/help/articles/activity_webhook)
to allow limited synchronizing of the edits made to Tracker stories back to the linked GitHub issue.
The following changes to the user story will be reflected back to the corresponding GitHub issue:

| When the story is...                                  | Then the linked GitHub issue is... |
| --------------------                                  | ---------------------------------- |
| Moved to the icebox                                   | Labeled `priority/undecided`       |
| Moved to the backlog                                  | Labeled `priority/backlog`         |
| Started                                               | Labeled `state/started`            |
| Finished                                              | Labeled `state/finished`           |
| Delivered                                             | Labeled `state/delivered`          |
| Rejected                                              | Labeled `state/rejected`           |
| Accepted                                              | Labeled `state/accepted`           |
| Made a Feature                                        | Labeled `enhancement`              |
| Made a Bug                                            | Labeled `bug`                      |
| Made a Chore                                          | Labeled `chore`                    |
| Made a Release                                        | Updated to remove `enhancement`, `bug`, `chore` labels |
| Estimated 0 points (all scales)                       | Labeled `estimate/XS`              |
| Estimated 1 point (all scales)                        | Labeled `estimate/S`               |
| Estimated 2 points (all scales)                       | Labeled `estimate/M`               |
| Estimated 3 points (only Fibonacci and Linear scales) | Labeled `estimate/L`               |
| Estimated 4 points (only Powers of 2 scale)           | Labeled `estimate/L`               |
| Estimated 5 points (only Fibonacci scale)             | Labeled `estimate/XL`              |
| Estimated 8 points (only Fibonacci and Powers of 2 scales) | Labeled `estimate/XXL`        |
| Un-estimated                                          | Updated the remove the above `estimate/*` labels |
| Assigned to an owner(s)                               | Updated to change the Assignees    |
| Unassigned                                            | Updated to clear the Assignees     |

If the user story is deleted, and the integration panel is refreshed,
then the issue will reappear in the integration panel. The Tracker story changes which
were previously automatically synchronized to the Github issue as described in the table above
are left unchanged on the GitHub issue. The issue can then be dragged and dropped back into
the backlog or icebox, and the synchronization described above will resume.

## Known Limitations

At this time, the app has the following limitations, which might be addressed by future enhancements:

- There is no authentication on the provided REST endpoints.
    - There is a read-only endpoint that could be exploited to read open issues from your GitHub repository. This
      is not a problem for public repositories, but think twice before using this for a private repository.
    - There is an endpoint which could be exploited to perform the edits described in the table above to your
      GitHub issues. Normally, even on a public repository, GitHub would prevent users who did not create the issue
      or who do not have write access to the repository from making these edits. Think twice about using this app
      if that is a concern for you.
- The app does not re-read configuration dynamically. When you change configuration you can
  restart the application's pod(s) using `rollout restart deployment/issues2stories`.
- Each running instance of issues2stories can only be configured to link a
  single GitHub repository to a single Tracker project. If you would like to
  use issues2stories for multiple Tracker projects, you would currently
  need to run multiple copies of it.
- The GitHub issue labels that the app manages are not configurable. However, they could be changed
  at compile time by editing the source.
  See the comments in [internal/trackeractivity/constants.go](internal/trackeractivity/constants.go)
  for more information.
- The GitHub issue labels that the app manages must be created manually in GitHub before using the app.
  See [internal/trackeractivity/constants.go](internal/trackeractivity/constants.go) for a list of
  label names that are assumed to exist on your GitHub repository.
- Aside from Fibonacci, linear, and powers of 2 estimate point scales, Tracker also supports "custom" scales.
  Custom scales are not supported by issues2stories unless the compile-time values for the point scale keys in
  [internal/trackeractivity/constants.go](internal/trackeractivity/constants.go) are adjusted to match your
  custom scale.
- The issues2stories Tracker story import integration will not work for the open GitHub issues beyond the first
  100 issues. This is because the issues2stories backend code is not using pagination yet when making the 
  GitHub API request to list open stories. This should be easy to fix.

## Installing

issues2stories is easily built as a container image.
It is an HTTP server which listens on a single port to provide several REST-style endpoints.
It can be run on any platform which can run the container image and provide HTTPS
ingress with working TLS certificates that are trusted by Pivotal Tracker.

### Optional: Configuring GitHub Usernames for Tracker Project Members

If you would like the Assignees of a GitHub issue to be automatically updated when the owners of the linked
Tracker story are updated, then you'll need to provide a little extra configuration so issues2stories
knows how to map your team's Tracker users to your GitHub users.

It's hard to find Tracker user IDs in the Tracker UI, so we'll use the
[Tracker "GET members" API](https://www.pivotaltracker.com/help/api/rest/v5#projects_project_id_memberships_get)
to find the user IDs of your project members.

1. Find the ID of your Tracker project. This is shown in the URL bar of your browser
   while you are viewing your Tracker project. e.g. `https://www.pivotaltracker.com/n/projects/2453999`
   is the project with ID `2453999`.
1. Copy your Tracker API token from your [Tracker profile page](https://www.pivotaltracker.com/profile).
   You may need to click the "Create New Token" button on that page if you have no token listed.
1. ```bash
   export TRACKER_TOKEN='abc123' # replace this example value with your actual API token
   export PROJECT_ID='2453999' # replace this number with your actual project ID
   curl -s -H "X-TrackerToken: $TRACKER_TOKEN" "https://www.pivotaltracker.com/services/v5/projects/$PROJECT_ID/memberships" | jq -r '[.[] | .person]'
   ```
   Note that if you have lots of members in your project, you may need to add the `limit` query
   parameter to get more responses in the list.
   See the [Tracker API pagination documentation](https://www.pivotaltracker.com/help/api#Paginating_List_Responses).
1. You'll get a list of values, where each value looks like this:
   ```json
    {
        "kind": "person",
        "id": 3344177,
        "name": "Ryan Richard",
        "email": "ryan@example.com",
        "initials": "RR",
        "username": "rr"
    }
    ```
1. Note the `id` value for each member. It is not necessary to provide configuration for every member. Members who
   are not configured will not be set as assignees on GitHub issues when they become owners of Tracker stories.
1. Craft a YAML map of Tracker user IDs to GitHub usernames for the people on your team.
   e.g. `{3344177: cfryanr, 1234567: some-other-github-username}`
1. Provide that map as the configuration value for ytt when deploying. See [deploy/values.yaml](deploy/values.yaml)
   and also see deployment example below.

### Example: Installing on [Google Kubernetes Engine (GKE)](https://cloud.google.com/kubernetes-engine)

The [deploy](deploy) directory contains [ytt](https://carvel.dev/ytt) templates
to deploy the app on GKE. With some modification, these could be customized to
deploy the app to other types of Kubernetes clusters, but that is beyond the
scope of this document.

1. Compile the app and build the container image. Push it to an image repository
   that will be accessible from your GKE cluster.
   [GCR repositories](https://console.cloud.google.com/gcr)
   in the same GCP project are automatically accessible from GKE clusters, so that's a
   convenient place to push the container image.

    ```bash
    # Assuming that you are already logged in to your gcloud CLI...
    GCP_PROJECT=your-gcp-project-name
    docker build . -t gcr.io/$GCP_PROJECT/issues2stories:latest
    gcloud auth configure-docker gcr.io
    docker push gcr.io/$GCP_PROJECT/issues2stories:latest
    ```

1. [Reserve a global static external IP address](https://console.cloud.google.com/networking/addresses/list).
   Create it using the settings `Premium` and `Global`.
   The `name` that you choose for this reservation will be used when rendering the ytt
   templates below.

1. [Create a DNS A record](https://console.cloud.google.com/net-services/dns/zones) in a DNS zone
   for the reserved address. For example, if your DNS zone is named `your-zone.com`, then
   create a record to map `issues2stories.your-zone.com.` to the IP address that you reserved above.
   Wait about 10-15 minutes for DNS propagation. Confirm that it has propagated with:

   ```bash
   nslookup issues2stories.your-zone.com
   ```

   This domain name will be used when rendering the ytt templates below.

1. Edit the values.yaml file or use ytt command-line options to provide values
   for all parameters in [deploy/values.yaml](deploy/values.yaml).
   Render the ytt template:

    ```bash
    cd deploy
    ytt --file . > /tmp/deployment.yaml
    ```

1. Make sure your current context for `kubectl` is your GKE Kubernetes cluster.
   Then install with:

    ```bash
    kapp deploy --app issues2stories --diff-changes --file /tmp/deployment.yaml
    ```

1. Look up the IP address of the load balancer with `kubectl get ingress -n issues2stories`.
   It should be the same IP address that you reserved in the previous step. Wait until
   the address appears.

1. Wait for the managed certificate to be provisioned. This may take up to 15 minutes.
   You can check on the status of the certificate with the following command:

   ```bash
   kubectl describe managedcertificate issues2stories -n issues2stories
   ```

   The `Status.CertificateStatus` field will be `Active` when it is finished.

1. Confirm that everything is working:

   ```bash
   curl https://issues2stories.your-zone.com/tracker_import
   ```

1. Add the integration to the Tracker project.
   In the project, navigate to "Integrations -> Add an Integration -> Other".
   Use the following settings:

   - Project: Choose the project
   - Name: `issues2stories`
   - Basic Auth Username: Leave blank
   - Basic Auth Password: Leave blank
   - Base URL: `https://github.com/your-org/your-repo/issues/`
   - Import API URL: `https://issues2stories.your-zone.com/tracker_import`
   - Enabled: Checked

1. Add the webhook to the Tracker project.
   In the project, navigate to "More -> Webhooks".
   Use the following settings to add a webhook:

   - URL: https://issues2stories.your-zone.com/tracker_activity

1. In your Tracker project, click on "issues2stories" (with the jigsaw puzzle icon)
   button in the left-hand side navigation. The panel will appear and should show
   a list of all open issues from your GitHub repository.
