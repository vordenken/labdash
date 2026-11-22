# Security

## Reporting a vulnerability

Use **Security → Report a vulnerability** on GitHub if private vulnerability
reporting is enabled. Otherwise ask the maintainer for a private reporting
channel without including technical details in a public issue. Include the
affected build or image digest, scanner and advisory ID if available. Do not
send passwords, cookies, signing keys, real layouts or internal hostnames.

## Deployment boundaries

- The Compose example is for local HTTP. For remote use, put an HTTPS reverse
  proxy in front of the service, set `LABDASH_SECURE_COOKIE=true`, and do not
  expose the backend port directly. Set `LABDASH_TRUSTED_PROXIES` to the
  proxy's address or network so login throttling sees the real client. Without
  it, `X-Forwarded-For` is ignored and all clients behind the proxy share one
  limit. The proxy must keep the original `Host` header; labdash rejects
  state-changing requests whose `Origin` does not match it.
- The browser loads nothing from other origins: labdash sends a Content
  Security Policy that restricts it to the dashboard itself. Only put icon
  files you trust into the icon folder.
- With `LABDASH_ICON_DOWNLOAD=true` (the default), the server downloads
  missing icons from fixed jsDelivr URLs for Dashboard Icons and Material
  Design Icons. Icon names cannot point the server at other hosts. Downloads
  require a signed-in session, are limited to 2 MiB, must look like an image
  and are served with a sandboxing policy. Set `LABDASH_ICON_DOWNLOAD=false`
  to make no outbound requests.
- This is a shared-password dashboard, not a multi-tenant service. Every
  authenticated user can read and edit the layout. With `LABDASH_AUTH=none`,
  all reachable clients can do so. Never store credentials in tiles or labels.
- Keep Docker access off unless needed. Raw socket access typically grants
  control over the host, even with a `:ro` mount. Disabling the dashboard's
  action buttons is not a Docker permission boundary.
- Protect the config, backup and environment file. Rotate the session signing
  key as well as the password if access is compromised; rotating the password
  alone does not invalidate already signed sessions.

## Before making a repository public

Review the files to be committed **and any existing Git history** for
credentials and private infrastructure. Enable private vulnerability reporting
and Dependabot alerts in the repository settings. Re-run the checks in
[Contributing](CONTRIBUTING.md) on the revision to be released. A clean
dependency scan is not a security review of an installation.
