# HTTP Authentication

This document describes how to configure HTTP authentication for downloads (bootstrap.json and item payloads) using mobileconfig and command-line flags.

## Mobileconfig keys

- `HTTPAuthUser` and `HTTPAuthPassword`
  - Enables HTTP Basic Auth
  - Applied to all HTTP requests when both are present

- `HTTPHeaders` (dict or array)
  - Adds arbitrary headers to all HTTP requests
  - Dict format example:
    ```xml
    <key>HTTPHeaders</key>
    <dict>
      <key>Authorization</key>
      <string>Bearer your-token</string>
      <key>X-API-Key</key>
      <string>abc123</string>
    </dict>
    ```
  - Array format example:
    ```xml
    <key>HTTPHeaders</key>
    <array>
      <dict><key>name</key><string>Authorization</string><key>value</key><string>Bearer your-token</string></dict>
      <dict><key>name</key><string>X-API-Key</string><key>value</key><string>abc123</string></dict>
    </array>
    ```

- `HeaderAuthorization` (convenience)
  - String value used to set `Authorization` header (e.g., `Bearer TOKEN`)

## Command-line flags

- `--headers "Bearer TOKEN"`
  - Convenience to set `Authorization: Bearer TOKEN`
- `--follow-redirects`
  - Follow 30x redirects (disabled by default)

## Precedence

1. Explicit CLI flags (when provided)
2. Mode-specific mobileconfig (e.g., `<key>daemon</key>` section)
3. Shared mobileconfig (`<key>shared</key>`)

If both Basic Auth and an `Authorization` header are present, the explicit `Authorization` header takes precedence.

## Security notes

- Prefer mobileconfig for credentials; avoid CLI for secrets
- Always use HTTPS
- Sensitive header values are never printed; verbose logs show header names only

## Troubleshooting

- Enable debug/verbose to see auth flow (headers redacted):
  ```bash
  sudo ./go-installapplications --debug --verbose --mode standalone
  ```
- Verify mobileconfig keys are under the correct domain and section (`shared` or per-mode)

