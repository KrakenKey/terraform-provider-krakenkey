# Contributing to terraform-provider-krakenkey

## Prerequisites

- Go 1.22+
- Terraform 1.5+ (for local development override)
- A KrakenKey account with an API key

## Building

```bash
git clone https://github.com/krakenkey/terraform-provider-krakenkey
cd terraform-provider-krakenkey
go build ./...
```

## Testing

```bash
go test ./...
```

Acceptance tests (requires `KRAKENKEY_API_KEY` and a verified domain):

```bash
KRAKENKEY_API_KEY=kk_... TF_ACC=1 go test ./internal/provider/... -v
```

## Local Development Override

To use a locally built provider instead of the Terraform Registry version, add a `dev_overrides` block to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/terraform-provider-krakenkey"
  }
  direct {}
}
```

Then run `go build -o terraform-provider-krakenkey` and use it directly with `terraform plan` (skip `terraform init`).

## Release

Releases are automated via GoReleaser on tag push. To cut a release:

1. Update `CHANGELOG.md`
2. Tag: `git tag vX.Y.Z && git push origin vX.Y.Z`
3. The CI pipeline builds multi-platform binaries, signs them, and publishes to the Terraform Registry.

## API Reference

The provider wraps the KrakenKey REST API. See [AGENTS.md](https://github.com/krakenkey/krakenkey/blob/main/AGENTS.md) in the main repo for the full endpoint reference, authentication guide, and plan limits.
