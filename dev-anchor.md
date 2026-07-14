---
type: Note
relationships:
  Type:
    - "[[note]]"
---

# DevAnchor

A portable, version-controlled system (`da`) that syncs a developer's entire working environment across machines — IDE settings, tool configs, shell config, credentials, and package lists — from a single Git repository.

## User story

1. User gets a new machine
2. Sets up basics — browser, system preferences, network
3. Runs DevAnchor
4. DevAnchor detects the OS, installs every dependency and tool, lays down all configs and symlinks, provisions certificates — machine is ready to develop on

## Core flow

- Bootstrap: user runs `da login` to authenticate, then `da pull <profile>` (or just `da` for the default profile). `da` detects the OS, decrypts secrets via the login session, installs missing tools, symlinks configs, and provisions certificates.
- Daily iteration: edit configs in place, `da anchor --name <profile>`, commit changes, push. On another machine, `da login` + `da pull` to apply.

## CLI overview

```
# Authenticate and decrypt secrets on a new machine
da login

# Anchor the current setup as a named profile
da anchor --name "frontend-stack"

# Pull and apply a profile on any machine
da pull frontend-stack
```

## Requirements

### Bootstrap flow

- Single entry point script (`./bootstrap` or `./install`) idempotent — safe to re-run
- Bootstrap must work with zero pre-installed dependencies beyond git (bootstrap itself installs the package manager and essential tools)
- Report what was changed (new symlinks, installed packages, config diffs) on each run
- Exit cleanly if git is not installed, with a link to install it

### Secret management

- SSH keys, API tokens, cloud credentials, and any `.env` files must never be stored in plaintext in the repo
- Non-sensitive config (editor themes, tool settings without credentials) stored as plain files, versioned normally
- Sensitive files encrypted with **`sops` + `age`** — provides field-level encryption so YAML/JSON keys stay readable while values are encrypted, making diffs useful: `git diff` shows *which* values changed, not the values themselves
- No backend to maintain — everything lives in git

**Key setup (one-time per user):**

```
# Generate an age keypair
age-keygen -o age.key

# Encrypt the private key with your password
age -p -o age.key.enc age.key

# Shred the plaintext key, commit the rest
rm age.key
git add age.pub age.key.enc
```

**On a new machine (`da login`):**

1. User is prompted for their password
2. `age -d age.key.enc` recovers the age private key into a session-scoped temp file
3. `sops` reads `.sops.yaml` (configured with the age public key) and uses the recovered key transparently
4. Decrypted files are laid down via symlinks; the temp key file is cleaned up after the session

**Properties of this approach:**

- Zero infrastructure — no auth server, no SaaS, no key escrow
- Works fully offline (only needs the git clone + the password)
- `sops` diff output is meaningful: `$MY_VAR` changed vs a blob of base64
- `.sops.yaml` maps which files are encrypted and which age key can decrypt them
- Selective encryption granularity: encrypt whole files or individual YAML values

**CI / headless flow:**

- `da login --token <pat>` or `$DA_TOKEN` env var (a personal access token used to derive or retrieve the password without interactive prompt)

**Key recovery:**

- Password is the single recovery secret — no external provider dependency
- If the password is lost, there is no recovery path (by design). Recommend storing the password in a password manager (1Password, Bitwarden) alongside other vault secrets.
- For team or org use, consider a threshold recovery scheme (Shamir) — but for a personal dev environment, a password manager is sufficient.

### Symlink strategy

- All config files live under a single repo root (e.g., `~/.dotfiles/`)
- Bootstrap creates symlinks from standard locations (`~/.zshrc`, `~/.config/`, etc.) to files in the repo
- Do not overwrite existing files unless they are already symlinks into the repo (diff and warn otherwise)
- Store a manifest of all symlinks to enable clean rollback

### Platform detection

- Target OSes: **macOS** + top 3 developer Linux distributions — **Ubuntu/Debian**, **Fedora**, **Arch Linux**
- Detect OS and distro at the start of bootstrap
- Map to default package manager — `brew` on macOS, `apt` on Debian/Ubuntu, `dnf` on Fedora, `pacman` on Arch
- Allow per-platform overrides for specific package entries
- Support WSL detection (Linux distros running under WSL on Windows are handled via their native package manager)

### Tool inventory

- Define tools declaratively — name, expected binary, source (brew formula, cargo crate, pip package, uv tool, npm global, GitHub release), expected version or `latest`
- Bootstrap reads the inventory, checks what's installed, and installs missing items
- Stores the installed version for drift detection

### Machine-specific overrides

- A `local/` directory loaded after common configs — files here are `.gitignore`-d
- Per-machine overrides for hostname-sensitive settings, display config, PATH additions, etc.
- An `overrides/` directory for shared but conditional configs keyed by machine name or hostname pattern

### Selective profiles

- Group tools into profiles (e.g., `work`, `personal`, `minimal`, `gaming`)
- Bootstrap accepts `--profile` flag; defaults to `minimal` (shell, git, editor)
- Profiles can depend on other profiles (e.g., `work` includes `minimal`)

### Dependency ordering

- Some tools must be installed before their configs are laid down (terminal → terminal theme)
- Bootstrap phases: (1) package manager + system deps, (2) tools, (3) symlinks, (4) post-install hooks
- Post-install hooks are shell scripts in the repo, run in order, idempotent

### Drift detection

- `./check` or `./status` subcommand compares installed versions against the manifest
- Detects: missing symlinks, modified config files outside the repo, tool version mismatch, missing tools
- Output a diff-compatible report for CI or manual inspection

### Rollback

- Every bootstrap run records the previous state of each symlink target in a `.backup/` directory
- `./rollback` restores the last backup and reverts the repo to the previous git commit
- `./rollback --step N` goes back N bootstrap runs

### Package-manager TOML support

- Define tools in a TOML manifest (e.g., `tools.toml`)
- Each entry: name, manager, package, version, profile, platform filter
- Bootstrap parser reads the manifest and routes each entry to the correct installer

### Use uv where possible

- Prefer `uv` over `pip` for Python tool installation (faster, isolated, deterministic)
- Fall back to `pip` only when the package is unavailable via `uv`
- Use `uv tool install` for CLI tools shipped as Python packages
