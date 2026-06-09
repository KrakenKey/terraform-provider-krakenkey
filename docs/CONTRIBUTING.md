# Contributing

Guidance for building, testing, and releasing the KrakenKey Terraform provider.

## Prerequisites

- Go 1.21+
- Terraform 1.5+
- A KrakenKey API key (`kk_...`) for acceptance tests

## Build

```bash
go build ./...
```

To install the provider locally for manual testing:

```bash
go install .
```

This places the binary at `$GOPATH/bin/terraform-provider-krakenkey`.

## Unit Tests

```bash
go test ./...
```

## Acceptance Tests

Acceptance tests create real KrakenKey resources. Set the following environment variables before running:

```bash
export TF_ACC=1
export KK_API_KEY=kk_...
export TF_LOG=INFO  # optional
```

Then run:

```bash
go test ./... -run TestAcc -v -timeout 30m
```

Acceptance tests may create billable resources. Clean up after running by checking your KrakenKey dashboard.

## Local Development Override

To use a locally built provider binary instead of the registry version, add a `dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/your/GOPATH/bin"
  }
  direct {}
}
```

With this in place, `terraform init` is not needed for the provider — Terraform picks up the local binary directly.

## Releases

Releases are created by pushing a version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The `release.yml` workflow runs GoReleaser, which cross-compiles for all supported platforms and creates a GitHub Release with the provider binaries and SHA-256 checksums.

## Versioning

This provider follows [Semantic Versioning](https://semver.org/):

- **Patch** (`v0.1.x`): bug fixes, documentation updates
- **Minor** (`v0.x.0`): new resources, new attributes, backwards-compatible changes
- **Major** (`vx.0.0`): breaking changes to existing resource schemas
