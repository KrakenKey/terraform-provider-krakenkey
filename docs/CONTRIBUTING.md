# Contributing

This document covers building, testing, and releasing the KrakenKey Terraform provider.

---

## Prerequisites

- Go >= 1.22
- Terraform >= 1.5 (for manual testing)
- A KrakenKey account with an API key

---

## Build

```bash
go build ./...
```

To install the provider locally for development:

```bash
go install .
```

Then add a `~/.terraformrc` dev override:

```hcl
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/your/GOPATH/bin"
  }
  direct {}
}
```

---

## Test

Unit tests (no API calls):

```bash
go test ./...
```

Acceptance tests (requires live API key — creates real resources):

```bash
export KRAKENKEY_API_KEY=your-api-key
TF_ACC=1 go test ./... -v -run TestAcc
```

---

## Release

Releases are created by pushing a version tag. CI handles cross-compilation and publishing to the Terraform Registry.

```bash
git tag v0.1.0
git push origin v0.1.0
```

Follow [semantic versioning](https://semver.org): `MAJOR.MINOR.PATCH`.

- **PATCH**: Bug fixes, no schema changes
- **MINOR**: New resources or arguments (backwards-compatible)
- **MAJOR**: Breaking schema changes or provider renames
