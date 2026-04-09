# Security Policy

## Responsible Disclosure

If you discover a security vulnerability, please report it privately by emailing the maintainers instead of opening a public issue. We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

Email: security@domuk-k.dev (or open a private GitHub Security Advisory on this repo)

## Known Security Considerations

### Docker Socket Access

OMA's Docker sandbox backend requires access to the Docker socket (`/var/run/docker.sock`). This effectively grants root-level access to the host. In production, consider:

- Running with a rootless Docker setup.
- Restricting which images the sandbox can use.
- Using network-isolated Docker environments.

### API Key Authentication

Set `OMA_API_KEY` to protect the OMA server API. Without it, the API is unauthenticated and should only be exposed on trusted networks.

### Local Sandbox Mode

The `local` sandbox type runs agent commands directly on the host without isolation. It is intended for **development only** and must not be used in production or on shared machines.

## Supported Versions

Only the latest release is actively maintained with security patches.
