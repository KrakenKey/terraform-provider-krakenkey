# Contributing

This document covers building, testing, and releasing the KrakenKey
Terraform provider.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Building](#building)
- [Running Unit Tests](#running-unit-tests)
- [Running Acceptance Tests](#running-acceptance-tests)
- [Local Development Override](#local-development-override)
- [Code Style](#code-style)
- [Releasing](#releasing)

---

## Prerequisites

| Tool | Version |
|---|---|
| Go | ≥ 1.22 |
| Terraform | ≥ 1.5 (for local manual testing) |
| GoReleaser | ≥ 2.0 (release builds only) |

---

## Building

```bash
git clone https://github.com/krakenkey/terraform-provider-krakenkey.git
cd terraform-provider-krakenkey
go build ./...
```

To produce a binary in the repo root (used by the local dev override):

```bash
go build -o terraform-provider-krakenkey .
```

---

## Running Unit Tests

```bash
go test ./...
```

Unit tests do not call the KrakenKey API and require no credentials.

---

## Running Acceptance Tests

Acceptance tests create real resources against the KrakenKey API. They are
gated behind the `TF_ACC` environment variable to prevent accidental runs.

```bash
export TF_ACC=1
export KK_API_KEY="kkkey_live_..."   # must be a valid live API key
go test ./... -v -run TestAcc -timeout 120m
```

> **Warning**: Acceptance tests create and destroy real KrakenKey resources.
> Use a dedicated test workspace and API key. Do not run against a production
> workspace.

Individual test targets follow the pattern `TestAcc<ResourceName>`, e.g.:

```bash
go test ./internal/provider -v -run TestAccDomain -timeout 30m
```

---

## Local Development Override

To use a locally built provider binary with an existing Terraform configuration
without publishing to a registry:

1. Build the binary into a directory of your choice:

   ```bash
   go build -o ~/terraform-providers/terraform-provider-krakenkey .
   ```

2. Add (or update) `~/.terraformrc`:

   ```hcl
   provider_installation {
     dev_overrides {
       "krakenkey/krakenkey" = "/Users/you/terraform-providers"
     }
     direct {}
   }
   ```

3. In your Terraform working directory, run `terraform apply` as normal.
   Terraform will use the local binary instead of fetching from the registry.
   Note: `terraform init` will warn about the dev override — this is expected.

---

## Code Style

- Run `gofmt -w .` before committing.
- Run `go vet ./...` and fix all warnings.
- Provider resource and data-source schemas must include `Description` fields
  for every attribute — these are used to generate the Terraform Registry
  documentation.
- Error messages must be user-actionable: include the API response body or
  the HTTP status where relevant.

---

## Releasing

Releases are automated via GoReleaser and triggered by a version tag on `main`.

1. Ensure `CHANGELOG.md` has an entry for the new version.
2. Merge all changes to `main`.
3. Tag the release:

   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. The GitHub Actions release workflow runs `goreleaser release` automatically.
   It builds cross-platform binaries, signs them with the KrakenKey GPG key,
   and publishes to the GitHub Releases page.
5. After a successful release, update the `required_providers` examples in
   `README.md` to reference the new version.

### Terraform Registry Publication

The provider is not yet published to the public Terraform Registry. Once the
first stable resource set ships (`v0.1.0`), follow HashiCorp's
[publishing documentation](https://developer.hashicorp.com/terraform/registry/providers/publishing)
to register the provider. The `RELEASES.md` will be updated at that point.
