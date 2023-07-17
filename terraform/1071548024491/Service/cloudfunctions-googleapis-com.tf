resource "google_project_service" "cloudfunctions_googleapis_com" {
  project = "1071548024491"
  service = "cloudfunctions.googleapis.com"
}
# terraform import google_project_service.cloudfunctions_googleapis_com 1071548024491/cloudfunctions.googleapis.com
