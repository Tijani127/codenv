# Changelog

All notable changes to codenv are recorded here.

Release codenames are Canadian provinces and territories: the major version is
the province, and each minor version names a place inside it (v1.0 Saskatchewan,
v1.1 Regina, v2.0 Manitoba, v2.2 Winnipeg). The map lives in
[.github/CODENAMES.txt](.github/CODENAMES.txt); the release workflow reads it,
so a tag that is not listed ships without a codename.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and codenv adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `codenv shellenv --compact` is advertised in `--help` but the flag was never
  read, so it silently printed the full multi-line script. It now emits a
  single `PATH` export for PowerShell, bash and cmd.
- `CODEENV_SHELL_FORMAT` was documented but not implemented. It now selects the
  `shellenv` format and falls back to auto-detection when the value is not
  recognised.
- `codenv rm --force` did nothing; `rm` already succeeds when a package is
  absent, so the flag has been removed.
- `LookPath` resolved relative paths against codenv's own working directory
  instead of the project's, so `codenv shell .\bin\app.exe` failed. Relative
  paths are now resolved against the project directory.
- An in-place environment filter used by `--pure` compacted a slice without
  shortening it, which leaked unrelated variables into the sanitised
  environment.

### Removed

- Dead code with no callers: `engine.lastLines`, `project.RelOrSame`,
  `ui.SetOutput`, `ui.SetColorEnabled`, `ui.KeyValue`, `ui.Rule`, and an
  unused `flagAddOutputs` variable.

### Documentation

- Reworked the README: the shim rationale and the store internals no longer
  overlap, the table of contents covers every section, the environment-variable
  table is split into variables codenv reads versus the ones it sets, and the
  devbox comparison no longer claims `devbox create` is unimplemented.

## [1.0.0] - 2026-10-06 - Saskatchewan

First stable release. codenv creates isolated, reproducible development
environments on Windows, backed by [Scoop](https://scoop.sh) instead of Nix,
with **shims disabled**.

### Why shims are the problem

Scoop makes every program reachable through one global shim directory and adds
that directory to your persistent user `PATH`. That works for a general-purpose
package manager and works badly for a reproducible dev environment: two
projects cannot run different versions of the same tool, the shim directory
leaks into unrelated terminals, and GUI packages quietly add Start Menu
shortcuts.

codenv takes a different approach. Scoop's shim files record the *resolved*
target of every binary it installs, so codenv reads them to learn the exact
directories a package contributes, records that in a per-store index, then
**deletes the shim directory**. `PATH` is composed from concrete
`apps/<pkg>/<version>` directories. Scoop's `current` junction is never used
for activation, so a background `scoop update` cannot change what an existing
environment resolves to.

### Added

**Environments**

- `codenv init` creates a `codenv.json`; `codenv add`/`rm`/`install`/`update`/
  `list` manage packages.
- `codenv.lock.json` pins exact versions, buckets and architectures. A pinned
  version that cannot be installed is a hard error, never a silent
  substitution, because an environment that quietly runs the wrong version is
  worse than one that says it cannot be built.
- **Multiple versions of one package coexist in a shared store.** Two projects
  can pin 7-Zip 24.09 and 26.03 at the same time, from one store.
- `store: "project"` gives a project a private store under `.codenv/store`.
- `include` merges other `codenv.json` files.
- Every install snapshots and restores your persistent `PATH`, `SCOOP_PATH`
  and `PSModulePath`, even when Scoop fails, and removes Start Menu shortcuts
  that an install created.

**Shell and running things**

- `codenv shell` opens an isolated shell with a `codenv:` prompt marker;
  `codenv shell <cmd>` runs one command inside it.
- `codenv run` executes a script from `codenv.json` or any command, propagating
  the child's exit code.
- `shell.init_hook` runs on every shell and script start.
- `--pure` reduces the environment to a minimal set, `--env`/`--env-file` set
  variables, `codenv shellenv` prints PowerShell, bash or cmd code for
  direnv and editor integrations.

**Global packages**

- `codenv global add/rm/list/install/update/run/shellenv/pull/push/path` manage
  a set of tools available to every project, in their own store.

**Services**

- `codenv services up/start/stop/restart/ls/logs/clean` supervise the services
  in `codenv-services.json`, natively, with no external process manager.
  Supports `depends_on`, per-service environment and working directories, and
  background or streaming-log operation.

**Secrets**

- `codenv secrets add/from/list/rm/download` declare which variables a project
  needs. **Values are never stored.** They are read from your environment when
  a shell or script starts, so `codenv.json` stays safe to commit.
- `secrets.from` remaps a secret onto other variables, trying each in order,
  which lets one project point at a developer's local database and another at
  CI's.
- A missing source variable is a hard error for `shell`/`run` and a warning for
  `shellenv`, because editor integrations invoke the latter unconditionally.
- `secrets download` redacts by default; `--values` is an explicit opt-in.

**Discovery and diagnostics**

- `codenv search` searches the local buckets offline.
- `codenv info` shows a manifest: version, licence, dependencies, binaries.
- `codenv doctor` checks the things that actually break: missing git, an
  unwritable store, bucket manifests left dirty by autoupdate, a stray shims
  directory, stale index entries, uninstalled pins, unset secret sources, and
  packages already on your system `PATH`. `doctor --fix` repairs what is safe
  to repair and never installs or removes packages.

**Project scaffolding and generation**

- `codenv create <template>` scaffolds a project with a pinned toolchain.
  Templates: `go`, `python`, `node`, `rust`, `minimal`.
- `codenv generate dockerfile|devcontainer|direnv|readme` produces supporting
  files.
- `codenv cache info|clean`, and shell completions for PowerShell, bash, zsh
  and fish.

### Known limitations

- **Windows only.** Scoop and Windows `PATH` semantics are the foundation. The
  code compiles elsewhere, but the tool cannot function there.
- **Requires git.** Scoop is a git checkout and codenv clones its buckets.
- **Historical version pins are limited by Scoop.** Resolving a version that is
  not the bucket's current one relies on Scoop's autoupdate, which downloads the
  release to compute hashes. Releases from before arm64 Windows existed
  (for example `ripgrep@14.1.1`) cannot be resolved this way.
- **Versioned Python is published as separate packages.** `python@3.11` is not a
  valid reference; use `versions/python311`. codenv detects this and says so.
- **The package dependency graph is effectively flat.** None of the 4,815
  manifests in `main`, `extras` and `versions` declare a `dependencies` key;
  modern Scoop uses `pre_install`/`installer` scripts instead.

### Not implemented

- Jetify Cloud equivalents: `auth`, and cloud `secrets`/`cache`. There is
  nothing to authenticate against locally.
- Devbox plugins, which describe Nix build setup. Scoop packages are prebuilt.
- `devbox create` style templates from a remote catalogue.
- Linux and macOS releases. `generate dockerfile` emits a **Windows** container
  Dockerfile, since a Linux image cannot run Scoop.

[Unreleased]: https://github.com/Tijani127/codenv/compare/v1.0...HEAD
[1.0.0]: https://github.com/Tijani127/codenv/releases/tag/v1.0