resource "google_artifact_registry_repository" "trusted_applet_builder" {
  description   = "Repo to host Docker images which build the Armored Witness Trusted Applet."
  format        = "DOCKER"
  location      = "europe-west2"
  project       = "armored-witness"
  repository_id = "trusted-applet-builder"
}
# terraform import google_artifact_registry_repository.trusted_applet_builder projects/armored-witness/locations/europe-west2/repositories/trusted-applet-builder
