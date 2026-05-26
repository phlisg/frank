# Contributing to Frank

Thanks for your interest in contributing!

## Getting Started

1. Fork the repo and clone your fork
2. Create a branch from `develop`
3. Make your changes
4. Run `go test ./...` — all tests must pass
5. If you changed any templates, regenerate golden files: `go test ./cmd/ -update`
6. Open a PR targeting `develop`

## What Makes a Good PR

- One concern per PR
- Include a short description of *why*, not just *what*
- Add or update tests for behavioral changes

## Reporting Bugs

Use the [bug report template](https://github.com/phlisg/frank/issues/new?template=bug_report.yml). Include your `frank.yaml`, Frank version, and steps to reproduce.

## Feature Requests

Use the [feature request template](https://github.com/phlisg/frank/issues/new?template=feature_request.yml). Describe the use case — the *why* matters more than the *how*.

## Code Style

- `go vet` and `staticcheck` must pass
- Follow existing patterns in the codebase
- No unnecessary abstractions

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
