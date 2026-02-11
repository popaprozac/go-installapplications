# Backwards Compatibility with InstallApplications

This document summarizes compatibility with [macadmins/installapplications](https://github.com/macadmins/installapplications) (Python) and notes from [installapplications-swiftly](https://github.com/MichalMMac/installapplications-swiftly) where relevant. go-installapplications intentionally deviates in some areas (e.g. optional hash, userscript path not required, different log paths); those are called out below.

## Original InstallApplications Reference

- **Repo**: https://github.com/macadmins/installapplications  
- **Main script**: `payload/Library/installapplications/installapplications.py`  
- **Stages**: `preflight` (single rootscript), `setupassistant`, `userland`  
- **JSON**: `bootstrap.json` at install path; key names: `file`, `name`, `type`, `url`, `hash`, `packageid`, `version`, `donotwait`, `skip_if`, `retries`, `retrywait`, and for packages **`required`** (Python code uses `item["required"]`, not `pkg_required`)

---

## Flags and Options

| Original IA (Python)     | go-installapplications                         | Notes |
|--------------------------|------------------------------------------------|-------|
| `--jsonurl`              | `--jsonurl`                                    | Same |
| `--iapath`               | `--iapath` (added) or `--installpath`, `--compat` | `--iapath` and `--installpath` set install path; `--compat` sets path to `/Library/installapplications` |
| `--laidentifier`         | `--laidentifier`                               | Same |
| `--ldidentifier`         | `--ldidentifier`                               | Same |
| `--reboot`               | `--reboot`                                     | Same |
| `--skip-validation`      | `--skip-validation`                            | Same; when false, existing `bootstrap.json` is removed before re-download (match original) |
| `--headers`              | `--headers`                                    | Authorization header (e.g. `Basic xxx`) |
| `--follow-redirects`     | `--follow-redirects`                           | Same |
| `--dry-run`              | `--dry-run`                                    | Same |
| `--userscript`           | N/A                                            | Original runs only user scripts (agent-style); we use `--mode agent` as a full IPC server instead |

---

## Bootstrap JSON

- **Filename**: `bootstrap.json` (same).  
- **Path**: `{InstallPath}/bootstrap.json`; with `--compat` or `--iapath /Library/installapplications` this matches original.  
- **Re-download**: When `--skip-validation` is false, existing `bootstrap.json` is **deleted before download** so the file is always refreshed (matches original).  
- **Item keys**: We accept both `pkg_required` and **`required`** (original IA uses `required`); either sets “always install / don’t skip by receipt”.

---

## Package Receipt and Version Semantics

- **Skip when already installed**: If `pkg_required` / `required` is false (default), we **skip** when the package is already installed and the **installed version ≥ required version** (loose comparison).  
- **Loose version comparison**: Implemented to match original (e.g. `10.6` and `10.6.0` are equal).  
- **When `pkg_required` is true**: We always run the install and do not skip based on receipt.

---

## Phases and Item Types

| Phase            | Original IA                         | go-installapplications |
|------------------|-------------------------------------|-------------------------|
| preflight        | Single rootscript; exit 0 = cleanup & exit | Same (single rootscript, exit 0 = cleanup & exit) |
| setupassistant   | package, rootscript                 | Same; userscript in setupassistant rejected (validation) |
| userland         | package, rootscript, userscript     | Same; plus `rootfile` / `userfile` (Swift-inspired) |

- **User script path**: Original recommends `file` under a `userscripts` subfolder (e.g. `/Library/installapplications/userscripts/...`). We do not require this; any path is allowed (similar to installapplications-swiftly).

---

## Per-Item Retries and Hash

- **Retries**: Per-item `retries` and `retrywait` (defaults 3 and 5 when unset) are supported and passed through to the download layer.  
- **Hash**: SHA256 verification is supported; if `hash` is empty we skip verification (original effectively required hash for downloads; we allow optional hash and document strict use in README if desired).

---

## Optional / Minor Differences

- **installer command**: Original uses `installer -verboseR -pkg ... -target /`. We use `installer -pkg ... -target /`. Adding `-verbose` for logging could be done for closer parity.  
- **Hash required**: For strict parity you could require `hash` when `url` is present; currently we allow missing hash.  
- **Log paths**: We use `/var/log/go-installapplications/` by default; original uses `/var/log/installapplications.log` and user log under `/var/tmp/installapplications/`. With `--compat` only the install path changes; log paths remain our defaults unless extended later.

---

## Summary of Code Changes (This Review)

1. **Package receipt logic**: Skip when installed version ≥ required (loose) and `pkg_required` is false; always install when `pkg_required` is true.  
2. **Loose version comparison**: `LooseVersionCompare` added so `10.6` and `10.6.0` are treated as equal.  
3. **Bootstrap re-download**: When `SkipValidation` is false, remove existing `bootstrap.json` before downloading.  
4. **`--iapath`**: New flag to set install path (same effect as `--installpath`) for original plist/script compatibility.  
5. **JSON `required`**: Bootstrap items accept both `pkg_required` and `required`; either sets “always install” (original IA uses `required`).

These updates keep behavior aligned with the original InstallApplications while preserving your existing enhancements (fail policy, parallel downloads, mobile config, etc.).
