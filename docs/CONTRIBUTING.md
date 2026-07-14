# Contributing

## Prerequisites

| Tool | Version |
|------|---------|
| [Go](https://golang.org/) | >= 1.22 |
| [Terraform](https://developer.hashicorp.com/terraform) | >= 1.5 |

## Build

```bash
go build ./...
```

The provider binary is output to the current directory.

## Unit Tests

Unit tests do not require credentials:

```bash
go test ./...
```

## Acceptance Tests

Acceptance tests create real KrakenKey resources. They require a valid API key and count against your plan limits.

```bash
export TF_ACC=1
export KK_API_KEY=kk_...
go test ./... -v -timeout 30m
```

> **Warning**: Acceptance tests create and delete real domains, certificates, and endpoints. Run against a test account, not production.

## Local Development Override

To test the provider against a local Terraform configuration without publishing to the registry:

```bash
# 1. Build the binary
go build -o terraform-provider-krakenkey ./...

# 2. Add a dev_overrides block to ~/.terraformrc
cat >> ~/.terraformrc <<'EOF'
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/absolute/path/to/provider/directory"
  }
  direct {}
}
EOF

# 3. In your Terraform config directory, run plan/apply directly (no init needed)
terraform plan
```

## Code Style

- Run `gofmt -w .` before committing.
- Run `go vet ./...` to catch common issues.
- All resource schema attributes must have a non-empty `Description` field.
- Sensitive values (API keys, certificate PEMs) must set `Sensitive: true` in the schema.
- Computed-only attributes must set `Computed: true`; required arguments must set `Required: true`.

## Release Process

1. Merge all changes to `main`.
2. Tag the release: `git tag v0.x.y && git push origin v0.x.y`.
3. GoReleaser builds and publishes the binaries to GitHub Releases automatically via CI.
4. Once published, the Terraform Registry picks up the new version within a few minutes.

Update the `required_providers` version constraint examples in `README.md` after each minor or major release.
