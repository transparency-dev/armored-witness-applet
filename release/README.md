# Trusted Applet Release Process

## File structure

*   The `trusted_applet/` directory contains the Dockerfile to build an image
    which installs dependencies and compiles the Trusted Applet with Tamago. The
    version of Tamago to use can be specified with the Docker
    [build arg](https://docs.docker.com/engine/reference/commandline/build/#build-arg)
    `TAMAGO_VERSION`.
*   The `json_constructor/` directory contains the Dockerfile and source files
    to build a Go helper binary to construct the Claimant Model Statement of the
    transparency log.

## Build and Release Process

A
[Cloud Build trigger](https://cloud.google.com/build/docs/automating-builds/create-manage-triggers)
is defined to with the cloudbuild.yaml config file and is invoked when a new
tag is published in this repository.

The pipeline includes two main steps: building and making available the Trust
Applet files, and writing the release to the transparency log.

1.  Cloud Build builds the Trusted Applet Docker image, copies the compiled
    Trusted Applet ELF file, signs it and creates a detached signature file.
    Then, it uploads both to a public Google Cloud Storage bucket.
1.  Cloud Build builds the JSON constructor binary Docker image, which runs the
    binary with arguments specific to this release. It then copies the output
    Statement and adds it to the public transparency log.

TODO: add links for the GCS buckets once public.

## Claimant Model

| Role         | Description |
| -----------  | ----------- |
| **Claimant** | Transparency.dev team |
| **Claim**    | <ol><li>The digest of the executable is derived from this source Github repository, and is reproducible.</li><li>The executable is issued by the Transparency.dev team.</li></ol> |
| **Believer** | Armored Witness devices |
| **Verifier** | <ol><li>For Claim #1: third party auditing the Transparency.dev team</li><li>For Claim #2: the Transparency.dev team</li></ol> |
| **Arbiter**  | Log ecosystem participants and reliers |

The **Statement** is defined in [api/log_entries.go](api/log_entries.go).
An example is available at
[api/example_firmware_release.json](api/example_firmware_release.json).