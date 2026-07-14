# DevAnchor

Take a fresh machine from bare `git` to fully provisioned: detect OS, install
tools, symlink dotfiles with interactive conflict resolution, decrypt
secrets, and reverse-sync local changes back into the repo.

DevAnchor is a single static Go binary (`da`) plus a POSIX `bootstrap`
script that downloads it. The git repo you clone **is** your DevAnchor — it
holds `tools.toml`, `symlinks.toml`, `configs/`, and (optionally) encrypted
secrets.

## Requirements

- `git`
- macOS, Ubuntu/Debian, Fedora, or Arch Linux (including under WSL)
- A package manager for your platform: `brew`, `apt`, `dnf`, or `pacman`
- Optional, for secrets: [`sops`](https://github.com/getsops/sops) and
  [`age`](https://github.com/FiloSottile/age)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/vanya1301/dev-anchor/develop/bootstrap | sh
```

This detects your OS/arch, downloads the matching `da` binary into
`~/.local/bin/da`, and runs `da pull`.

Or build from source:

```sh
git clone git@github.com:vanya1301/dev-anchor.git
cd dev-anchor
go build -o da ./cmd/da
./da pull
```

Or grab a prebuilt binary from a recent CI run (see [CI](#ci) below).

## Commands

| Command | Description |
|---|---|
| `da pull [profile]` | Detect OS, install missing tools from `tools.toml`, symlink configs from `symlinks.toml`, decrypt secrets if `.sops.yaml` is present. |
| `da anchor [path]` | Reverse-sync: capture newly installed tools and (optionally) adopt a new config file at `path` into `configs/`, re-encrypting secrets. |
| `da login` | Decrypt the `age` key for this shell session (prompts for password, or reads `$DA_TOKEN`). |
| `da keygen` | Generate a new `age` keypair, password-encrypt the private key, and write `age.pub` + `age.key.enc`. |

Global flag: `--yes` — non-interactive, defaults every symlink conflict
prompt to "replace" instead of prompting.

## Repo layout

```
tools.toml       # declarative tool inventory (see below)
symlinks.toml    # maps configs/ files to their home-directory targets
configs/         # the actual dotfiles, symlinked into place
.sops.yaml       # sops encryption rules (optional, only if using secrets)
age.pub          # your public age key (safe to commit)
age.key.enc      # your password-encrypted private age key (safe to commit)
.da/             # local state (gitignored) — tool state, symlink manifest
.backup/         # backups of files replaced during symlink conflicts (gitignored)
```

### `tools.toml`

```toml
[[tool]]
name = "ripgrep"
manager = "brew"      # brew | apt | dnf | pacman | uv | cargo | npm | go
package = "ripgrep"
version = "latest"
binary = "rg"          # binary checked on $PATH to decide if install is needed
platform = ["macos", "linux"]   # omit for all platforms

[tool.overrides]        # per-manager package name overrides
apt = "ripgrep"
dnf = "ripgrep"
pacman = "ripgrep"
```

`da pull` installs any tool whose `binary` isn't already on `$PATH`. Python
CLIs prefer `uv tool install`; `github-release` sources are not
auto-installed yet.

### `symlinks.toml`

```toml
[[link]]
source = "zsh/.zshrc"   # relative to configs/
target = "~/.zshrc"     # ~ expands to $HOME
```

`da pull` symlinks each target to `configs/<source>`. If the target already
exists as a real file, you're prompted:

```
a) leave as-is
b) copy into configs/, keep file
c) copy into configs/, replace with symlink  [recommended]
```

`--yes` picks (c) automatically.

### Secrets

Run `da keygen` once to generate `age.pub` / `age.key.enc`, then reference
your public key in `.sops.yaml`:

```yaml
creation_rules:
  - path_regex: secrets/.*\.yaml$
    age: age1...
```

`da pull` prompts for your password (or reads `$DA_TOKEN` non-interactively)
and decrypts matching files for the session. `da login` establishes a
session without running the full pull. `da anchor` re-encrypts before you
commit. The recovered key is written with mode `0600` to a session temp
file and shredded on close — plaintext secrets are never committed.

## Development

```sh
go test ./...
go build -o da ./cmd/da
```

All subprocess calls (`brew`, `sops`, `age`, etc.) route through
`internal/runner.CommandRunner`, so every package is unit-testable with the
included `MockRunner`.

## CI

Every push to `develop` runs `go test ./...` and builds `da` for
`darwin`/`linux` × `amd64`/`arm64`. Binaries are uploaded as workflow
artifacts (`da_<os>_<arch>`) — download them from the
[Actions tab](https://github.com/vanya1301/dev-anchor/actions) on a
successful run.

## Status

MVP. Not yet implemented: `da drift`, `da rollback`, profiles, local
per-machine overrides, post-install hooks, `github-release` auto-install.
