resource "google_project_service" "logging_googleapis_com" {
  project = "1071548024491"
  service = "logging.googleapis.com"
}
# terraform import google_project_service.logging_googleapis_com 1071548024491/logging.googleapis.com
