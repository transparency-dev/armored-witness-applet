resource "google_kms_crypto_key" "trusted_applet" {
  destroy_scheduled_duration = "86400s"
  key_ring                   = "projects/armored-witness/locations/europe-west2/keyRings/armored-witness"
  name                       = "trusted-applet"
  purpose                    = "ASYMMETRIC_SIGN"

  version_template {
    algorithm        = "EC_SIGN_P256_SHA256"
    protection_level = "SOFTWARE"
  }
}
# terraform import google_kms_crypto_key.trusted_applet projects/armored-witness/locations/europe-west2/keyRings/armored-witness/cryptoKeys/trusted-applet
