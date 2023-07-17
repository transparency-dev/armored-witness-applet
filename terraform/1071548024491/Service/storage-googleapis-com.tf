resource "google_project_service" "storage_googleapis_com" {
  project = "1071548024491"
  service = "storage.googleapis.com"
}
# terraform import google_project_service.storage_googleapis_com 1071548024491/storage.googleapis.com
