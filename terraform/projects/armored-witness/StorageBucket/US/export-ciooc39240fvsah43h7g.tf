resource "google_storage_bucket" "export_ciooc39240fvsah43h7g" {
  force_destroy               = false
  location                    = "US"
  name                        = "export-ciooc39240fvsah43h7g"
  project                     = "armored-witness"
  public_access_prevention    = "inherited"
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
}
# terraform import google_storage_bucket.export_ciooc39240fvsah43h7g export-ciooc39240fvsah43h7g
