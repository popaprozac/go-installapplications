# generatejson

Generate JSON configuration files for installapplications.

## Usage

```bash
go run main.go --base-url URL --output PATH \
  [--compat | --install-path /Library/go-installapplications] \
  --item "key=value ..." [--item "..."]
```

## Example

```bash
go run main.go --base-url https://github.com --output ~/Desktop \
  --item "item-name=preflight item-path=/localpath/preflight.py item-stage=preflight item-type=rootscript item-url=https://github.com/preflight/preflight.py script-do-not-wait=false pkg-skip-if=false retries=0 retrywait=0 required=false" \
  --item "item-name=setupassistant_package item-path=/localpath/package.pkg item-stage=setupassistant item-type=package item-url=https://github.com/setupassistant/package.pkg script-do-not-wait=false pkg-skip-if=false retries=5 retrywait=10 required=false" \
  --item "item-name=userland_user_script item-path=/localpath/userscript.py item-stage=userland item-type=userscript item-url=https://github.com/userland/userscript.py script-do-not-wait=true pkg-skip-if=false retries=0 retrywait=0 required=false"
```

## Item Parameters

Each `--item` requires exactly 10 key=value pairs:

- `item-name=NAME` - Display name
- `item-path=PATH` - Local file path  
- `item-stage=STAGE` - preflight, setupassistant, or userland
- `item-type=TYPE` - package, rootscript, userscript, rootfile, userfile
- `item-url=URL` - Download URL (empty = auto-generate)
- `script-do-not-wait=BOOL` - true/false
- `pkg-skip-if=ARCH` - intel, arm64, or false  
- `retries=INT` - Retry count
- `retrywait=INT` - Retry delay seconds
- `required=BOOL` - true/false

Outputs `bootstrap.json` to specified directory.

### Notes
- `--compat` forces original IA paths under `/Library/installapplications` (userscripts in `/Library/installapplications/userscripts`).
- Without `--compat`, paths default to `/Library/go-installapplications`. You can override with `--install-path`.
- For `package` items, JSON uses `pkg_required` in output. Supply `required=...` in CLI and it is mapped to `pkg_required`.
- URL auto-generation uses the basename of `item-path`: `{base-url}/{stage}/{basename(item-path)}`.
- For `rootfile`/`userfile`, `item-path` is treated as the destination path and is emitted as `file` as-is.
