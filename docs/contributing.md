# Contributing

[← Back to README](../README.md)

## Running locally

```bash
go build -o frank .
./frank
```

For live reload during development:

```bash
go tool air
```

## Running tests

```bash
go test ./...
```

## Releasing a new version

Frank uses tag-based releases. Pushing a tag triggers the GitHub Actions workflow, which builds binaries for all platforms and creates a GitHub release.

```bash
git tag v1.2.3
git push origin v1.2.3
```

The version is injected into the binary at build time — `frank version` will return the tag name.
