# issues2stories

issues2stories is an app which provides a limited amount of synchronizing between
[Pivotal Tracker](https://www.pivotaltracker.com/) user stories in a Tracker project
and GitHub issues in a GihHub repository.

issues2stories provides a
Pivotal Tracker [integration](https://www.pivotaltracker.com/help/articles/other_integration)
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
Changes to the user story are not reflected in the corresponding GitHub issue.

If the user story is deleted, and the integration panel is refreshed,
then the issue will reappear in the integration panel.

## Installing

issues2stories is easily built as a container image.
It is an HTTP server which listens on a single port to provide several REST-style endpoints.
It can be run on any platform which can run the container image and provide HTTPS
ingress with working TLS certificates that are trusted by Pivotal Tracker.

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

1. Add the integration to the Tracker project. Integrations -> Add an Integration -> Other.
   Use the following settings:

   - Project: Choose the project
   - Name: `issues2stories`
   - Basic Auth Username: Leave blank
   - Basic Auth Password: Leave blank
   - Base URL: `https://github.com/your-org/your-repo/issues/`
   - Import API URL: `https://issues2stories.your-zone.com/tracker_import`
   - Enabled: Checked

1. In your Tracker project, click on "issues2stories" (with the jigsaw puzzle icon)
   button in the left-hand side navigation. The panel will appear and should show
   a list of all open issues from your GitHub repository.
