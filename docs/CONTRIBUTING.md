# Contributing to terraform-provider-krakenkey

## Prerequisites

- [Go](https://golang.org/doc/install) ≥ 1.22
- [Terraform](https://developer.hashicorp.com/terraform/downloads) ≥ 1.5 (for acceptance tests)
- A KrakenKey account with a live API key (for acceptance tests only)

## Build

```bash
go build ./...
```

The provider binary is output to the module root. See [Local Dev Override](#local-dev-override) to wire it into Terraform.

## Unit Tests

```bash
go test ./...
```

Unit tests do not require network access or a KrakenKey account.

## Acceptance Tests

Acceptance tests create real KrakenKey resources and require live credentials.

```bash
export TF_ACC=1
export KK_API_KEY="kkkey_live_..."

go test ./... -timeout 30m
```

> Run acceptance tests against a staging or test account. Resources created during testing are destroyed at the end of each test run, but failures mid-run may leave orphaned resources.

## Local Dev Override

1. Build the binary:
   ```bash
   go build -o terraform-provider-krakenkey .
   ```

2. Add `dev_overrides` to `~/.terraformrc`:
   ```hcl
   provider_installation {
     dev_overrides {
       "krakenkey/krakenkey" = "/absolute/path/to/provider/directory"
     }
     direct {}
   }
   ```

3. Skip `terraform init` in your test configuration — Terraform will use the local binary directly.

## Code Style

- Format with `gofmt` before committing.
- Run `go vet ./...` and resolve all warnings.
- Every schema attribute must have a non-empty `Description` field.
- Sensitive attributes (API keys, private keys) must set `Sensitive: true` in the schema.
- Follow the [Terraform Plugin Framework best practices](https://developer.hashicorp.com/terraform/plugin/framework/best-practices).

## Release Process

Releases are built and published automatically by [GoReleaser](https://goreleaser.com/) when a version tag is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The GitHub Actions release workflow:
1. Runs `goreleaser release`
2. Signs the artifacts with the KrakenKey GPG key
3. Publishes the release to the [Terraform Registry](https://registry.terraform.io/providers/krakenkey/krakenkey)

### Versioning Policy

This provider follows [Semantic Versioning](https://semver.org/):

| Change type | Version bump |
|---|---|
| New resource or data source | Minor (`0.x.0`) |
| New optional argument on existing resource | Minor |
| Breaking argument rename or removal | Major (`x.0.0`) |
| Bug fix with no schema change | Patch (`0.0.x`) |
