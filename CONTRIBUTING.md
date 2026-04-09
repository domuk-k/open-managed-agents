# Contributing to OMA

Thanks for your interest in contributing to open-managed-agents!

## Development Setup

1. **Go 1.22+** with CGO enabled (required for SQLite).
2. Clone the repo and run:
   ```bash
   make build   # produces bin/oma
   make test    # runs all tests
   ```
3. Docker is optional but needed for container sandbox tests.

## Code Style

- Run `go fmt ./...` before committing.
- Run `go vet ./...` to catch common issues.
- The CI pipeline enforces `golangci-lint`; run `make lint` locally to check.

## Pull Request Process

1. Fork the repository.
2. Create a feature branch from `main` (`git checkout -b feat/my-feature`).
3. Write tests for new functionality.
4. Ensure `make test` and `make lint` pass.
5. Open a pull request against `main` with a clear description.

## Issues

Please use the GitHub issue templates when reporting bugs or requesting features.

## License

By contributing you agree that your contributions will be licensed under the MIT License.
