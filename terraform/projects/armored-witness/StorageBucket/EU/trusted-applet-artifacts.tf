resource "google_storage_bucket" "trusted_applet_artifacts" {
  force_destroy               = false
  location                    = "EU"
  name                        = "trusted-applet-artifacts"
  project                     = "armored-witness"
  public_access_prevention    = "inherited"
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
}
# terraform import google_storage_bucket.trusted_applet_artifacts trusted-applet-artifacts
