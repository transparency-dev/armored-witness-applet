resource "google_project_service" "containerregistry_googleapis_com" {
  project = "1071548024491"
  service = "containerregistry.googleapis.com"
}
# terraform import google_project_service.containerregistry_googleapis_com 1071548024491/containerregistry.googleapis.com
