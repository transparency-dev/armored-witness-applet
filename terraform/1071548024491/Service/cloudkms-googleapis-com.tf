resource "google_project_service" "cloudkms_googleapis_com" {
  project = "1071548024491"
  service = "cloudkms.googleapis.com"
}
# terraform import google_project_service.cloudkms_googleapis_com 1071548024491/cloudkms.googleapis.com
