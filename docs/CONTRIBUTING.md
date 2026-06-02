# Contributing

## Prerequisites

- Go 1.21+
- Terraform v1.5+
- A KrakenKey account and API key (for acceptance tests)

## Build

```bash
go build ./...
```

The provider binary is named `terraform-provider-krakenkey`.

## Unit Tests

```bash
go test ./...
```

## Acceptance Tests

Acceptance tests create real KrakenKey resources and incur API usage. Set `TF_ACC=1` and provide credentials:

```bash
export TF_ACC=1
export KRAKENKEY_API_KEY=kk_...
go test ./... -v -timeout 30m
```

## Local Development Override

To test the provider locally without publishing to the Terraform Registry, add a `dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/terraform-provider-krakenkey"
  }
  direct {}
}
```

Then build and place the binary in the path specified:

```bash
go build -o /path/to/terraform-provider-krakenkey .
```

Terraform will use the local binary instead of downloading from the registry. Note: `terraform init` will warn about the dev override.

## Release

Releases are automated via GoReleaser on tag push:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The GitHub Actions release workflow builds multi-platform binaries, creates the GitHub Release, and generates the `SHA256SUMS` file required by the Terraform Registry. The registry picks up new releases automatically via the GitHub webhook.

### Versioning

This provider follows [Semantic Versioning](https://semver.org/):

- **Patch** (`v0.1.x`): Bug fixes, no schema changes
- **Minor** (`v0.x.0`): New resources or data sources, backward-compatible attribute additions
- **Major** (`vx.0.0`): Breaking schema changes or removed resources

## Code Style

- Run `go vet ./...` and `golangci-lint run` before submitting a PR.
- Use the Terraform Plugin Framework (not the older SDK v2) for all new resources.
- Resource acceptance tests must use `resource.Test` with `CheckDestroy` to clean up resources on failure.
