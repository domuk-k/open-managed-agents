# @oma/cli-linux-arm64

Platform-specific binary package for OMA CLI (Linux ARM64).

This package is part of the `@oma/cli` distribution. It contains the precompiled
`oma` binary for Linux on ARM64 (aarch64).

## How it works

At release time, GoReleaser builds the `oma` binary for linux/arm64 and copies
it into this package directory before publishing to npm. The main `@oma/cli`
package declares this as an optional dependency and resolves the binary at runtime.

You should not install this package directly. Instead, install `@oma/cli`:

```bash
npm install -g @oma/cli
```
