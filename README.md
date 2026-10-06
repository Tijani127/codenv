# codenv

[![build](https://github.com/Tijani127/codenv/actions/workflows/build.yml/badge.svg)](https://github.com/Tijani127/codenv/actions/workflows/build.yml)
[![release](https://github.com/Tijani127/codenv/actions/workflows/release.yml/badge.svg)](https://github.com/Tijani127/codenv/actions/workflows/release.yml)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Instant, isolated, reproducible development environments on Windows.**

`codenv` is a Windows reimplementation of [Jetify's devbox](https://github.com/jetify-com/devbox), backed by
[Scoop](https://scoop.sh) instead of Nix — and with **shims disabled**. You declare your toolchain in
`codenv.json`, and every machine gets the exact same package versions placed directly on `PATH`, without
shims, without touching your system `PATH`, and without leaving Start Menu shortcuts behind.

```
codenv init
codenv add ripgrep 7zip
codenv shell
```

---

## Table of contents

- [Why shims are the wrong tool here](#why-shims-are-the-wrong-tool-here)
- [How it works](#how-it-works)
- [Install](#install)
- [Quick start](#quick-start)
- [Configuration reference](#configuration-reference)
- [Commands](#commands)
- [Store modes and version pinning](#store-modes-and-version-pinning)
- [What codenv guarantees](#what-codenv-guarantees)
- [Differences from devbox](#differences-from-devbox)
- [Environment variables](#environment-variables)
- [How this was verified](#how-this-was-verified)

---

## Why shims are the wrong tool here

Scoop makes every installed program reachable through a single directory of shims, and it adds that directory
to your **persistent user `PATH`**. That is excellent for a general-purpose package manager and wrong for a
reproducible dev environment:

| Problem | codenv's answer |
| --- | --- |
| One global shim dir, so two projects cannot run different versions of the same tool | `PATH` points at the exact `apps/<name>/<version>` directory per project, and multiple versions coexist in the store |
| The shim dir is permanently on your `PATH`, so it leaks into unrelated terminals | The shims directory is **deleted** after every install; your `PATH` is snapshotted and restored around every Scoop call |
| GUI packages silently add Start Menu shortcuts | Shortcuts created during an install are removed again |
| Installing upgrades the version every other project sees | Versions are pinned in `codenv.lock.json` and materialised per store |

Concretely: `codenv` lets one project use 7-Zip **24.09** while another uses **26.03** at the same time,
on the same machine, from one shared store, with no shims involved.

---

## How it works

```
%LOCALAPPDATA%\codenv\          CODEENV_HOME
└── store\                      shared store (default)
    ├── apps\<pkg>\<version>\   installed packages; `current` is a junction we never use
    ├── buckets\<name>\         cloned Scoop buckets (main, extras, versions)
    ├── cache\                  download cache
    └── .codenv-index.json      harvested bin directories, per package@version
└── global\store\               separate store for `codenv global`
```

1. **Bootstrap.** On first use, `codenv` clones Scoop and the default buckets into the store using `git`.
2. **Resolve.** Package specs (`ripgrep`, `ripgrep@14.1.1`, `extras/vcredist2022`) are resolved against the
   local bucket manifests and written to `codenv.lock.json` as exact versions.
3. **Install.** `scoop install … -u` runs inside a wrapper that snapshots the persistent user `PATH`,
   `SCOOP_PATH` and `PSModulePath`, then restores them afterwards — even if Scoop fails.
4. **Harvest, then delete.** Scoop writes shim files recording the *resolved* target of every binary.
   `codenv` reads those files to learn exactly which directories a package contributes, stores them in
   `.codenv-index.json`, then **deletes the entire shims directory**.
5. **Activate.** `codenv shell` builds `PATH` from the harvested directories (falling back to the manifest's
   `bin` entries) and launches a child shell with that environment. Nothing is persisted.

Because `PATH` is built from concrete version directories, `codenv` never depends on the `current` junction
that Scoop maintains.

---

## Install

Requires **Windows**, **PowerShell 5.1 or PowerShell 7**, and **git** (used to clone Scoop and its buckets).

Download the latest release and unzip it, or build from source:

```powershell
git clone https://github.com/Tijani127/codenv.git
cd codenv
go build -o codenv.exe .
```

The binary is self-contained; drop it anywhere on your `PATH`. Windows x64 and arm64 are both published.

Check your setup at any time with `codenv doctor`.

### Releases

Releases are named after Canadian provinces and territories. The **major version is the province**, and each
**minor version is a place inside it**:

| Tag | Codename | |
| --- | --- | --- |
| `v1.0` | Saskatchewan | |
| `v1.1` | Regina | Saskatchewan |
| `v1.4` | Saskatoon | |
| `v2.0` | Manitoba | |
| `v2.2` | Winnipeg | Manitoba |
| `v3.0` | Alberta | |
| `v4.0` | British Columbia | |

To cut a release: add the tag and codename to `.github/CODENAMES.txt`, commit, then push the tag. The
release workflow reads that file, so a tag that is not listed simply ships without a codename.

---

## Quick start

```powershell
# 1. Scaffold a project (or use 'codenv init' in an existing directory)
codenv create go myapp
cd myapp

# 2. Declare your toolchain
codenv add ripgrep 7zip
codenv add versions/python313   # a versioned package
codenv add extras/vcredist2022  # name a bucket

# 3. Use it
codenv shell                    # interactive shell, prompt marked with "codenv:"
codenv run rg --version         # one-off command in the environment
codenv shellenv | Out-String    # export the environment into an existing shell
```

`codenv create` writes `codenv.json`, a `README.md` and a `.gitignore`, and pins the toolchain each
template needs. List them with `codenv create templates`.

`codenv.json` and `codenv.lock.json` should both be committed; `.codenv/` should be git-ignored:

```gitignore
.codenv/
```

---

## Configuration reference

```jsonc
{
  // List form, or map form: { "ripgrep": "14.1.1" }
  "packages": ["ripgrep", "7zip@24.09", "extras/python@3.13.2"],

  // Extra buckets to clone into the store, beyond main/extras/versions
  "buckets": ["sysinternals"],

  // "shared" (default) reuses one store across projects; "project" isolates
  // this project completely so version pins can never collide.
  "store": "shared",

  // Environment variables set inside the codenv environment
  "env": { "NODE_ENV": "development" },

  // Variables whose values come from your environment, never from this file.
  // "names" are the variables a project needs; "from" remaps one to another.
  "secrets": {
    "names": ["API_KEY", "DATABASE_URL"],
    "from": { "DATABASE_URL": "PG_URL,PG_URL_FALLBACK" }
  },

  // Merge other codenv.json files (paths relative to this file)
  "include": ["../shared-toolchain/codenv.json"],

  "shell": {
    // Runs on every `codenv shell` and `codenv run`
    "init_hook": "$env:PATH = \"$env:PATH;C:\\tools\"",
    "scripts": {
      "test": "pytest -q",
      "lint": "golangci-lint run"
    }
  }
}
```

`codenv.lock.json` is generated. Do not edit it by hand; it records the exact version, bucket, architecture
and dependency graph that `codenv install` materialised.

Services live in `codenv-services.json` (same shape as devbox's `devbox-services.json`):

```jsonc
{
  "services": {
    "api": {
      "command": "npm run dev",
      "environment": ["PORT=3000"],
      "is_daemon": true
    },
    "db": { "command": "docker run --rm -p 5432:5432 postgres:16" }
  }
}
```

---

## Commands

| Command | Purpose |
| --- | --- |
| `codenv doctor` | Diagnose the setup; `--fix` repairs what is safe to repair |
| `codenv create <template> [dir]` | Scaffold a project (`go`, `python`, `node`, `rust`, `minimal`) |
| `codenv init [dir]` | Create just `codenv.json` |
| `codenv add <pkg>...` | Add packages and install them |
| `codenv install` | Install everything in `codenv.json` |
| `codenv rm <pkg>...` | Remove packages (`--purge` also deletes from the store) |
| `codenv update` | Update unpinned packages to latest (`--all` re-resolves pins too) |
| `codenv list` | Show packages, versions and install status |
| `codenv shell [cmd...]` | Interactive shell, or run one command inside the environment |
| `codenv run <script\|cmd>` | Run a script from `codenv.json`, or any command |
| `codenv shellenv` | Print shell code that applies the environment |
| `codenv search <query>` | Search the local buckets |
| `codenv info <pkg>...` | Show a manifest: version, licence, dependencies, binaries |
| `codenv global <sub>` | `add`, `rm`, `list`, `install`, `update`, `run`, `shellenv`, `pull`, `push`, `path` |
| `codenv services <sub>` | `up`, `start`, `stop`, `restart`, `ls`, `logs`, `clean` |
| `codenv secrets <sub>` | `add`, `from`, `list`, `rm`, `download` — declare which variables are secret |
| `codenv generate <target>` | `dockerfile`, `devcontainer`, `direnv`, `readme` |
| `codenv cache <sub>` | `info`, `clean` |
| `codenv completion <shell>` | `powershell`, `bash`, `zsh`, `fish` |
| `codenv version` | Version and store information |

Useful flags: `-c/--config <dir>`, `-q/--quiet`, `--pure`, `--env KEY=VALUE`, `--env-file <path>`,
`--format powershell|bash|cmd`, `--no-install`.

---

## Store modes and version pinning

**Multiple versions of the same package coexist in one store.** Two projects can pin different versions of the
same tool at the same time:

```
$ codenv -c ./project-a shell 7z     # 7-Zip 24.09
$ codenv -c ./project-b shell 7z     # 7-Zip 26.03
```

Each environment's `PATH` points at a concrete `apps\<pkg>\<version>` directory, so nothing interferes.
Installing a new version never disturbs a project that pinned an older one, and a background
`scoop update` cannot change what an existing environment resolves to.

### `"store": "shared"` (default)

One store for all projects, with per-project versions selected via `PATH`. Fast and deduplicated: the download
is shared, and only the directories differ. This is what you want unless you have a reason not to.

### `"store": "project"`

A private store under `.codenv/store`, fully isolated from every other project. Useful when you want a
project's packages to be removable without affecting others. The cost is disk space, since nothing is shared.

```jsonc
{ "packages": ["7zip@24.09"], "store": "project" }
```

### Historical versions

Pinning to a version other than the bucket's current version relies on Scoop's *autoupdate*, which downloads
the release to compute its hashes. Three caveats:

- It only works if that release publishes assets for every architecture Scoop probes. Packages from before
  arm64 Windows builds existed (for example `ripgrep@14.1.1`) cannot be resolved this way.
- Autoupdate rewrites the bucket manifest in place. `codenv` resets every bucket to a pristine git state after
  each install, so this can never leak a downgrade into another project's unpinned resolution.
- Versioned Python is published as separate packages (`python311`, `python312`, ...), so `python@3.11` is not a
  valid reference. `codenv` detects this and tells you what to use instead.

If a pinned version genuinely cannot be installed, `codenv` **fails the command** rather than substituting a
different version:

```
error: could not install the exact versions this project pins: main/ripgrep@14.1.1
codenv will not substitute a different version, because that would break reproducibility
```

That is deliberate. A reproducible environment that silently runs the wrong version is worse than one that
tells you it cannot be built.

---

## What codenv guarantees

- **No system `PATH` pollution.** The persistent user `PATH` is snapshotted before every Scoop invocation and
  restored afterwards, including on failure. Start Menu shortcuts created by an install are removed.
- **No shims.** The shims directory is deleted after every install, and `PATH` is composed from real package
  directories.
- **Version fidelity.** `PATH` references `apps\<pkg>\<version>`, never the `current` junction, so a
  background `scoop update` cannot change what an existing environment resolves to. Multiple versions of a
  package coexist in the store, so one project can pin an older version without affecting another.
- **No silent substitution.** If an exact pinned version cannot be installed, the command fails instead of
  quietly running something else.
- **Reproducibility.** `codenv.lock.json` pins exact versions; `codenv install` materialises exactly those.
- **Idempotence.** Re-running `install` reuses what is already present and does nothing when nothing changed.
- **Exit-code fidelity.** `codenv run` and `codenv shell <cmd>` propagate the child's exit code.

---

## Secrets

`codenv` never stores secret values. You declare **which** variables a project needs, and codenv picks the
values up from your environment when a shell or script starts. Nothing sensitive is ever written to disk or
committed.

```powershell
codenv secrets add API_KEY DATABASE_URL   # declare them
codenv secrets from DATABASE_URL PG_URL   # read DATABASE_URL from $PG_URL
codenv secrets list                       # shows set/unset, never values
```

```jsonc
"secrets": {
  "names": ["API_KEY", "DATABASE_URL"],
  "from": { "DATABASE_URL": "PG_URL,PG_URL_FALLBACK" }
}
```

- `from` remaps a secret onto other variables, trying each in order and using the first one that is set.
  That is how you point one project at a developer's local database and another at CI's.
- If a mapped source variable is unset, `codenv shell` and `codenv run` **fail** rather than starting a shell
  where your tooling would silently misbehave. `codenv shellenv` only warns, because direnv and IDE
  integrations invoke it unconditionally and a hard failure there would break your editor.
- `codenv secrets download` prints values **redacted** by default; `--values` is an explicit opt-in.
- Values come from the ambient environment, so anything that can set an environment variable can inject one.
  On a shared machine, prefer a per-project `--env-file` over relying on global variables.

---

## Troubleshooting

Start with:

```powershell
codenv doctor          # diagnose
codenv doctor --fix    # repair what codenv can safely repair
```

`doctor` checks the things that actually go wrong: missing `git`, an unwritable store, bucket manifests left
dirty by Scoop's autoupdate, a leftover shims directory, index entries for packages no longer on disk, pinned
versions that are not installed, unset secret sources, and package directories that are already on your system
`PATH`. It exits non-zero only when something genuinely blocks codenv.

`--fix` is deliberately conservative. It removes a stray shims directory, resets dirty bucket manifests, and
prunes dead index entries. It never installs or uninstalls packages, because that needs your judgement — it
points you at `codenv install` instead.

---

## Differences from devbox

`codenv` mirrors devbox's behaviour and command surface. These are deliberate differences:

| devbox | codenv | Why |
| --- | --- | --- |
| Nix store, content-addressed | Scoop store, versions kept side by side | Different package manager. Reproducibility comes from the lockfile rather than content addressing. |
| `devbox.json` + `devbox.lock.json` | `codenv.json` + `codenv.lock.json` | Same model, different names |
| `devbox secrets` (Jetify Cloud) | `codenv secrets` (local, no cloud) | Secrets are declared, not stored: values come from your environment at shell start. No account required. |
| `devbox auth`, `devbox cache upload` | not implemented | Jetify Cloud services. There is nothing to authenticate against locally. |
| Devbox plugins (jsonnet) | not implemented | Plugins describe Nix build setup; Scoop packages are prebuilt and need none. |
| `devbox generate dockerfile` → Linux image | `codenv generate dockerfile` → **Windows** container image | Scoop cannot run in a Linux container. |
| `devbox services` (process-compose) | `codenv services` (built-in supervisor) | No external process manager to install on Windows. Same config file shape. |
| `devbox create <template>` | not implemented | devbox templates are Nix-flavoured. |
| `devbox add --platform / --exclude-platform` | not implemented | Only one platform exists. |

Two structural notes about Scoop that shape the implementation:

- **The dependency graph is effectively flat.** Of the 4,815 manifests in `main`, `extras` and `versions`,
  **zero** declare a `dependencies` key — modern Scoop uses `pre_install`/`installer` scripts instead. The
  dependency support in `codenv` is therefore correct but rarely exercised. Harvesting Scoop's own shim
  metadata is what makes `PATH` complete, since it reflects the real on-disk layout including `persist`
  directories.
- **`git` is required**, because Scoop is a git checkout and `codenv` clones it and the buckets.

---

## Environment variables

| Variable | Purpose |
| --- | --- |
| `CODEENV_HOME` | Override the codenv home directory (default `%LOCALAPPDATA%\codenv`) |
| `CODEENV_SHELL` | Set to `1` inside a codenv environment |
| `CODEENV_PROJECT_DIR` | Absolute path of the active project |
| `CODEENV_GLOBAL` | Set to `1` inside a `codenv global` environment |
| `CODEENV_STORE` | Store root backing the current environment |
| `CODEENV_PATH_PREFIX` | The package directories `codenv` prepended to `PATH` |
| `CODEENV_POWERSHELL` | Force a specific PowerShell executable |
| `CODEENV_SHELL_BIN` | Force the shell `codenv shell` launches |
| `CODEENV_SHELL_FORMAT` | Default format for `shellenv` output |
| `NO_COLOR`, `CODEENV_FORCE_COLOR` | Colour control |

---

## How this was verified

The shimless approach is not theoretical. It was validated end to end against real Scoop packages before being
implemented, and the checks below are reproducible:

- After `codenv install`, the store has **no `shims` directory** at all.
- `rg --version` inside `codenv shell` reports the version pinned in `codenv.lock.json`, not the version on the
  ambient system `PATH`.
- The persistent user `PATH` is byte-identical before and after any `codenv` command, including failed ones.
- `codenv --pure` reduces the environment from 96 variables to 36 while the packages still resolve.
- Two projects hold different versions of the same tool simultaneously (`7zip@24.09` and `7zip@26.03`).
- Bucket repositories are left pristine (`git status` clean) after every install.

## License

MIT
