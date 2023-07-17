resource "google_project_service" "pubsub_googleapis_com" {
  project = "1071548024491"
  service = "pubsub.googleapis.com"
}
# terraform import google_project_service.pubsub_googleapis_com 1071548024491/pubsub.googleapis.com
