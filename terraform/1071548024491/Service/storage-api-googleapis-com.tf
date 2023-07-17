resource "google_project_service" "storage_api_googleapis_com" {
  project = "1071548024491"
  service = "storage-api.googleapis.com"
}
# terraform import google_project_service.storage_api_googleapis_com 1071548024491/storage-api.googleapis.com
