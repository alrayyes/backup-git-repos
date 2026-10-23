# backup-git-repos

[![Test](https://github.com/alrayyes/backup-git-repos/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/test.yml)
[![Codecov](https://codecov.io/gh/alrayyes/backup-git-repos/graph/badge.svg)](https://codecov.io/gh/alrayyes/backup-git-repos)
[![Lint](https://github.com/alrayyes/backup-git-repos/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/lint.yml)
[![Prose](https://github.com/alrayyes/backup-git-repos/actions/workflows/prose.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/prose.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/alrayyes/backup-git-repos.svg)](https://pkg.go.dev/github.com/alrayyes/backup-git-repos)
[![Release](https://img.shields.io/github/v/release/alrayyes/backup-git-repos)](https://github.com/alrayyes/backup-git-repos/releases)
[![Licence](https://img.shields.io/github/license/alrayyes/backup-git-repos)](LICENSE)

Backs up every repository on a self-hosted GitLab or Forgejo instance, or a
GitHub.com account, to a local, restorable copy.

A backup only counts if it survives the forge going away, so what
`backup-git-repos` keeps is a bare mirror clone of each repository: every
branch, every tag, and the same namespace folder structure the forge used. It
tells archived repositories from active ones, and can back up either set, both,
or write either out as a `.tar.gz` alongside the mirror.

## Features

- Mirrors every branch, tag and ref, not just the default branch
- Keeps the forge's own namespace structure on disk
- Filters by archived, active, or all repositories
- Refreshes existing mirrors incrementally instead of re-cloning
- Optionally writes archived, active, or all repositories out as `.tar.gz`,
  a gzipped copy of the bare mirror itself -- not a working-tree checkout,
  so every branch and tag survives the archive too
- Backs up several forges in one run: GitLab instances, Forgejo instances,
  GitHub.com accounts, or a mix
- Optionally deletes a mirror once its repository is gone upstream
  (`--prune-removed`), so a years-long backup tree doesn't keep every
  deleted or renamed repository forever
- Fetches Git LFS content alongside the mirror, for repositories that use it
  -- verified against a real Forgejo and GitLab instance; expected to work
  the same way against GitHub.com, since it authenticates the same way, but
  not verified live, since GitHub.com has no self-hostable equivalent to
  test against (see [CONTRIBUTING.md](CONTRIBUTING.md#tests))
- On GitLab, backs up each project's wiki and snippets too -- they're their
  own git repositories, never returned by the projects API a project itself
  comes from, so a run mirrors them as their own entries alongside the
  project rather than silently missing them
- Optionally exports a repository's issues, releases, or pull/merge
  requests -- including uploaded release assets and review comments --
  alongside its mirror (`--export-metadata issues,releases,pull-requests`)
  -- metadata a bare git mirror never captures on its own, since none of it
  lives in the git history

## Requirements

- **Go 1.26** or newer to build it.
- **git**, on `PATH`, to do the actual cloning. The tool shells out to it
  rather than re-implementing the protocol, which is what makes an incremental
  mirror refresh fast and the resulting `.git` directory exactly what `git
clone` produces anywhere else.
- **`git-lfs`**, on `PATH`, only if any repository you're backing up uses Git
  LFS. A mirror clone brings along every commit and pointer file on its own,
  but the LFS-tracked file contents live outside the object store git keeps
  for everything else, fetched separately; the tool detects LFS usage per
  repository and only reaches for `git-lfs` -- and only fails if it's
  missing -- when a repository actually needs it, so backing up nothing but
  ordinary repositories never needs it installed at all.
- A personal access token for each forge you back up, with read access to
  every repository you want. See [Configuration](#configuration) for where it
  goes.
- Somewhere to write the backup tree. It grows to roughly the size of every
  repository you're backing up, twice over if you also enable `.tar.gz`
  archives.

Working on the tool needs more than running it does; that list is in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Reports

Every push to `main` publishes its test and coverage reports:

- [Test results](https://apis.ryankes.eu/backup-git-repos/reports/tests/junit.xml)
  (JUnit XML)
- [Coverage](https://apis.ryankes.eu/backup-git-repos/reports/coverage/) (HTML),
  with [`coverage.xml`](https://apis.ryankes.eu/backup-git-repos/reports/coverage/coverage.xml)
  (Cobertura) and
  [`coverage.out`](https://apis.ryankes.eu/backup-git-repos/reports/coverage/coverage.out)
  (Go profile) beside it

## Installation

See [INSTALL.md](INSTALL.md) for every install option: the AUR, `.deb`/`.rpm`,
Docker, Nix, `go install`, and from source.

## Configuration

List every forge in a YAML file, the token included. That's the default
and recommended form: paste the token straight into `token`, and there's
nothing else to wire up before `run` works. Treat a config carrying one as
a secret, the same care you'd give any file with credentials in it —
`chmod 600` it, and don't check it in. If you'd rather keep the token out
of the file entirely, `token_env` is the opt-in for that; see [docs/configuration.md](docs/configuration.md).

`backup-git-repos config init` writes a starter file to get from a blank
directory to something you can edit — `$XDG_CONFIG_HOME/backup-git-repos/config.yaml`
(or `~/.config/backup-git-repos/config.yaml` if `XDG_CONFIG_HOME` isn't set)
by default, or wherever `--config`/`-c` names. It refuses to overwrite an
existing file unless you pass `--force`.

`run` and `list` read from that same default path when you don't pass
`--config` yourself, so once `config init` has written it, plain
`backup-git-repos run` is enough. `--config` still wins when you pass it,
and without it and with nothing at the default path either, the tool
offers to run `config init` right there when stdin looks like an
interactive terminal, or just exits telling you which path it checked
when it doesn't -- a script or a CI job never gets stuck waiting on a
prompt it can't answer.

```bash
backup-git-repos config init
```

```yaml
dest: /srv/backups/git
forges:
  - name: work # becomes the top-level folder for this forge's repos
    kind: gitlab
    url: https://gitlab.example.com
    token: glpat-...
  - name: home
    kind: forgejo
    url: https://git.example.org
    token: ...
  - name: personal
    kind: github
    token: ghp_...
```

`url` is only for a self-hosted forge; GitHub.com is the one instance, so a
`github` entry never sets it. Token scopes, `token_env`, `token_command` and
`skip_mirrors` are in [docs/configuration.md](docs/configuration.md).

## Usage

```bash
backup-git-repos run --config ./config.yaml
```

Resulting layout:

```text
/srv/backups/git/
  work/group/subgroup/repo.git/           # bare mirror, active repo
  home/team/repo.git/
  archive/work/group/subgroup/repo.tar.gz # only when --archive is set
  archive/home/team/old-repo.tar.gz       # archived repo selected by --archive:
                                           # only the tar.gz, no .git alongside it
  home/team/repo.metadata/issues/42.json  # only when --export-metadata issues is set
  home/team/repo.metadata/releases/v1.2.0/release.json    # only when --export-metadata releases is set
  home/team/repo.metadata/pull-requests/7.json  # only when --export-metadata pull-requests is set
```

- Every flag, with its environment variable: [docs/flags.md](docs/flags.md)
- Getting a repository back out of a mirror or archive:
  [docs/restoring.md](docs/restoring.md)
- Exporting issues, releases and pull requests:
  [docs/metadata-export.md](docs/metadata-export.md)

`backup-git-repos list` runs the same discovery without cloning anything, and
`run --dry-run` also says what each repository would do. Both are the fast way
to check a config picks up what you expect.

## License

This project is licensed under the GNU General Public License v3.0 — see the
[LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome. Open a pull request.

Read [CONTRIBUTING.md](CONTRIBUTING.md) first. It covers the setup, how the
test suites are split, and what each linter is for.
