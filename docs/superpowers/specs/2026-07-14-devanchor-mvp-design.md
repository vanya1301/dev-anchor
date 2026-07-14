# DevAnchor MVP — Design

Date: 2026-07-14
Source PRD: `dev-anchor.md`

## Summary

DevAnchor (`da`) is a portable, version-controlled system that syncs a
developer's working environment across machines from a single Git repository:
tool installation, config symlinks, and encrypted secrets.

This document specifies the **MVP vertical slice** that delivers the core user
story end-to-end: fresh machine → detect OS → install tools → symlink configs →
decrypt secrets. Later plans cover drift detection, rollback, profiles,
machine-specific overrides, and post-install hooks.

## Decisions

| Area | Decision |
|------|----------|
| CLI language | **Go** — single static binary, trivial cross-compile, fast build |
| Binary delivery | POSIX `bootstrap` script detects OS/arch, downloads prebuilt `da` from GitHub release, hands off |
| Secrets | Shell out to `sops` + `age` binaries (installed early in bootstrap) |
| Repo model | **Single repo** — the git repo *is* the DevAnchor (holds `tools.toml`, `configs/`, `.sops.yaml`, encrypted keys, `bootstrap`) |
| User-facing commands | Only `bootstrap`, `da <subcommand>`, and `git commit`/`push`. All crypto/sops/age orchestration hidden behind `da` |

## MVP Scope

**In:** bootstrap delivery, platform detection, `tools.toml` install, symlink
strategy with interactive conflict resolution, `da login`/`da keygen` secrets
lifecycle, **`da anchor` reverse-sync (capture installed tools + new configs +
encrypt secrets)**, "what changed" reporting.

**Out (deferred to later plans):** drift detection command (`da check`/`status`),
rollback (`da rollback`), selective profiles, machine-specific overrides
(`local/`, `overrides/`), post-install hooks, per-manager TOML extras beyond the
core inventory schema.

> Note: The MVP *writes* an installed-version state file (`.da/state.json`) and a
> symlink manifest (`.da/manifest.json`) so later drift/rollback plans have the
> data they need, but the drift and rollback *commands* are not implemented here.

## Architecture

```
dev-anchor/
  bootstrap                 # POSIX sh: git check, detect OS/arch, curl da binary, exec `da pull`
  cmd/da/main.go            # CLI entrypoint (cobra)
  internal/
    platform/               # OS + distro + arch + WSL detection → PkgManager selection
    pkgmgr/                 # installers + list-installed queries: brew/apt/dnf/pacman + uv/cargo/npm/go/github-release
    inventory/              # tools.toml parse + install orchestration + state store
    symlink/                # symlink create, safety diff/warn, conflict prompts, manifest
    secrets/                # keygen/login/anchor/decrypt via sops+age, session temp key
    reporter/               # "what changed" summary per run
    runner/                 # CommandRunner seam (subprocess abstraction, mockable)
  tools.toml                # template inventory
  symlinks.toml             # template symlink map
  .sops.yaml                # age key mapping (template)
```

**Design for isolation:** each `internal/` package has one purpose and a defined
interface. All subprocess calls go through `runner.CommandRunner`, so every
package is unit-testable with a mocked runner and `t.TempDir()`.

## Data Model

### `tools.toml`

```toml
[[tool]]
name     = "ripgrep"
manager  = "brew"          # brew|apt|dnf|pacman|uv|cargo|npm|go|github-release
package  = "ripgrep"       # manager-specific identifier
version  = "latest"        # or a pinned semver, e.g. "14.1.0"
binary   = "rg"            # presence check + drift comparison target
platform = ["macos","linux"]   # optional filter; omit = all platforms

[tool.overrides]           # optional per-manager package-name remap
apt = "ripgrep"
```

- Parsed with `github.com/BurntSushi/toml`.
- `binary` drives presence check (`exec.LookPath`) and version comparison
  (`<binary> --version`, parsed loosely, compared to `version` when pinned).
- `platform` filters entries; `overrides` remap the package id per manager.
- After install, the resolved version is recorded to `.da/state.json`.

### `symlinks.toml`

```toml
[[link]]
source = "zsh/.zshrc"      # path relative to configs/
target = "~/.zshrc"        # ~ expanded to $HOME
```

### `.da/state.json` and `.da/manifest.json`

- `state.json`: `{ tool_name: { version, manager, installed_at } }`.
- `manifest.json`: list of `{ target, source, backup_path?, created_at }` for every
  symlink created or changed, plus every backup taken.

## Symlink Strategy

Source of truth is `configs/`. For each `symlinks.toml` entry, resolve target and
apply:

- **Target missing** → create symlink, record to manifest.
- **Target is a symlink into the repo** → skip (idempotent).
- **Target is a real file** → **prompt** with a warning and three choices:
  - **a) Leave as-is** — no symlink; record as skipped-conflict in the report.
  - **b) Copy into `configs/`, keep file as-is** — adopt content into repo but
    leave the real file in place (no symlink created).
  - **c) Copy into `configs/`, replace with symlink** *(recommended, default)* —
    back up original to `.backup/`, copy content into `configs/`, create symlink.

Non-interactive mode (`--yes` / CI): default to **(c)**; if `--no-overwrite` is
set, default to **(a)**.

Prompt:

```
⚠  ~/.zshrc exists and is a real file (not a DevAnchor symlink).
   a) leave as-is
   b) copy into configs/, keep file
   c) copy into configs/, replace with symlink  [recommended]
choice [c]:
```

## Secrets

All key/crypto lifecycle is owned by `da`. Users never invoke `age`/`sops`
directly.

### `da keygen` (one-time)

1. Ensure `age`/`sops` present (installed in bootstrap phase 2).
2. Shell out to `age-keygen` to generate a keypair in memory/temp.
3. Prompt for a password; produce `age.key.enc` (password-encrypted private key)
   and `age.pub`.
4. Write/update `.sops.yaml` mapping the age public key.
5. Shred the plaintext private key; stage `age.pub`, `age.key.enc`, `.sops.yaml`.
   The user commits (git action stays with the user; no crypto CLI knowledge
   required).

### `da login`

1. Prompt for password (or read a PAT via `--token` / `$DA_TOKEN` for
   CI/headless, used to derive the password non-interactively).
2. `da` decrypts `age.key.enc` → session temp key file at
   `$XDG_RUNTIME_DIR` (fallback `os.MkdirTemp`), mode `0600`.
3. `da` sets `SOPS_AGE_KEY_FILE` for its own `sops` subprocess calls.
4. On process exit (`defer`) the temp key is shredded and removed.

### `da pull` (decrypt step)

For each encrypted file in `.sops.yaml`, `da` runs `sops -d` → writes plaintext to
a runtime location (`0600`) → symlinks into the target. Plaintext is never written
back into the repo.

## Reverse-Sync: `da anchor`

`da anchor` captures changes a user made on the local machine back into the
DevAnchor repo, so they can be committed and applied elsewhere. It handles three
categories:

### 1. Newly installed tools

For the active package manager, `da` lists user-installed packages and diffs
against `tools.toml`:

| Manager | List command |
|---------|--------------|
| brew | `brew leaves` |
| apt | `apt-mark showmanual` |
| dnf | `dnf repoquery --userinstalled --qf '%{name}'` |
| pacman | `pacman -Qe` |
| cargo | `cargo install --list` |
| npm | `npm ls -g --depth=0 --json` |
| uv | `uv tool list` |

Packages present locally but absent from `tools.toml` are shown; the user
interactively selects which to add. `da` writes new `[[tool]]` entries with the
resolved `manager`, `package`, `binary`, and pinned `version`.

### 2. Config changes

- **Edited existing configs** — files symlinked into `configs/` are edited in
  place, so the change is *already* in the repo. No `anchor` action needed;
  `git status` shows the diff.
- **New/unlinked configs** — the user points `da anchor <path>` at a file. `da`
  runs the same adopt flow as symlink conflict resolution choice (c): back up the
  original, copy content into `configs/`, add a `symlinks.toml` entry, and replace
  the file with a symlink.

### 3. Secrets

`da anchor` encrypts files flagged in `.sops.yaml` by running `sops -e`
internally. The user never types `sops`.

After `da anchor`, the user reviews with `git diff` and runs
`git commit`/`push` (git actions stay with the user).

## Bootstrap Phases

`bootstrap` (POSIX sh):

1. Check git is installed; if not, exit cleanly with an install link.
2. Detect OS/arch.
3. Download the matching prebuilt `da` binary from the GitHub release.
4. `exec da pull "$@"`.

`da pull` runs ordered, idempotent phases (safe to re-run):

1. **Platform detect** — OS/distro/arch/WSL → select package manager
   (`brew` macOS, `apt` Debian/Ubuntu, `dnf` Fedora, `pacman` Arch).
2. **Package manager + system deps** — ensure the manager exists; install `sops`,
   `age`, `uv`.
3. **Tools** — read `tools.toml`, install missing entries, record versions to
   `.da/state.json`. Prefer `uv tool install` for Python CLIs; fall back to `pip`.
4. **Symlinks** — apply `symlinks.toml` with the conflict-resolution flow above.
5. **Secrets** — establish a `da login` session, decrypt and symlink secrets.
6. **Report** — summary of installed packages, new/changed symlinks, conflicts,
   and decrypted files.

## Error Handling

- Each phase reports failures but continues where safe; a failed tool install does
  not abort symlinking.
- Fatal (abort) conditions: git missing, platform undetectable, wrong password.
- All subprocess calls route through `runner.CommandRunner` for consistent error
  capture and mockability.

## Testing Strategy

- Unit tests per `internal/` package using a mocked `CommandRunner` and
  `t.TempDir()`.
- Table-driven tests for platform detection across macOS/Ubuntu/Fedora/Arch/WSL.
- Symlink tests exercise all four target states (missing, repo-symlink, real-file
  ×3 choices) against a temp directory.
- Secrets tests mock `age`/`sops` invocations and assert temp-key cleanup.
- Anchor tests mock package-manager list commands and assert correct `tools.toml`
  diff/additions and the new-config adopt flow.
- A containerized smoke test running `bootstrap` per distro is planned for CI
  (out of MVP code scope, tracked for a later plan).

## Platform Support

macOS, Ubuntu/Debian, Fedora, Arch Linux, and those Linux distros under WSL
(handled via their native package manager).
