resource "google_project_service" "cloudbuild_googleapis_com" {
  project = "1071548024491"
  service = "cloudbuild.googleapis.com"
}
# terraform import google_project_service.cloudbuild_googleapis_com 1071548024491/cloudbuild.googleapis.com
