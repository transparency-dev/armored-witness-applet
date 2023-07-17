resource "google_project" "armored_witness" {
  auto_create_network = true
  billing_account     = "010691-72AE1D-B5D9C4"
  folder_id           = "1085079117341"
  name                = "armored-witness"
  project_id          = "armored-witness"
}
# terraform import google_project.armored_witness projects/armored-witness
