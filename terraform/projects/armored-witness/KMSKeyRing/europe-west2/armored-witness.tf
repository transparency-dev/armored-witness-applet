resource "google_kms_key_ring" "armored_witness" {
  location = "europe-west2"
  name     = "armored-witness"
  project  = "armored-witness"
}
# terraform import google_kms_key_ring.armored_witness projects/armored-witness/locations/europe-west2/keyRings/armored-witness
