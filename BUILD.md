# Building and Packaging

Two simple ways to produce a signed installer package. Pick one.

## Option A: Makefile (recommended)

Prereqs: Go toolchain, [munkipkg](https://github.com/munki/munki-pkg), Apple Developer ID certificates.

1) Set signing identity and version in `build-info.json`:

```json
{
  "version": "1.0.0",
  "signing_info": {
    "identity": "Developer ID Installer: Your Name (XXXXXXXXXX)",
    "timestamp": true
  }
}
```

2) Build and package (universal by default):

```bash
make build     # builds the binary
make package   # runs munkipkg and outputs to build/
```

Other targets exist if you need architecture-specific builds: `build-intel`, `build-arm`, `package-intel`, `package-arm`.

## Option B: Direct munkipkg

Prereqs: Same as above. Ensure `build-info.json` exists at repo root (used by the Makefile and packaging scripts).

```bash
# 1) Build the binary
go build -ldflags "-s -w" -trimpath -o go-installapplications .

# 2) Stage into payload
mkdir -p payload/Library/go-installapplications
cp go-installapplications payload/Library/go-installapplications/
chmod +x payload/Library/go-installapplications/go-installapplications

# 3) Create a signed package
munkipkg .   # outputs to build/
```

## Notes

- LaunchDaemon/Agent plists and install scripts are preconfigured. If you change labels/paths, update payload files before packaging.
- Postinstall ensures `/var/log/go-installapplications` exists with appropriate permissions for agent logging.
- For manual testing, you can tee logs with `--log-file`; in production, rely on launchd `StandardOutPath`/`StandardErrorPath`.
