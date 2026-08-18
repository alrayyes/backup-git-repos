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
- Optionally writes archived, active, or all repositories out as `.tar.gz`
- Backs up several forges in one run: GitLab instances, Forgejo instances,
  GitHub.com accounts, or a mix

## Requirements

- **Go 1.26** or newer to build it.
- **git**, on `PATH`, to do the actual cloning. The tool shells out to it
  rather than re-implementing the protocol, which is what makes an incremental
  mirror refresh fast and the resulting `.git` directory exactly what `git
clone` produces anywhere else.
- A personal access token for each forge you back up, with read access to
  every repository you want. See [Configuration](#configuration) for where it
  goes.
- Somewhere to write the backup tree. It grows to roughly the size of every
  repository you're backing up, twice over if you also enable `.tar.gz`
  archives.

Working on the tool needs more than running it does; that list is in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Installation

```bash
go install github.com/alrayyes/backup-git-repos/cmd/backup-git-repos@latest
```

Or build from a clone:

```bash
git clone https://github.com/alrayyes/backup-git-repos.git
cd backup-git-repos
go build -o backup-git-repos ./cmd/backup-git-repos
```

Released binaries are attached to each [GitHub
release](https://github.com/alrayyes/backup-git-repos/releases).

## Configuration

List every forge in a YAML file. The recommended form keeps a token out of
the file, naming the environment variable holding it instead — the tool
reads the variable at startup and fails before touching the network if it's
unset. A literal token in the file is also supported; see below.

`backup-git-repos config init` writes a starter file to get from a blank
directory to something you can edit — `$XDG_CONFIG_HOME/backup-git-repos/config.yaml`
(or `~/.config/backup-git-repos/config.yaml` if `XDG_CONFIG_HOME` isn't set)
by default, or wherever `--config`/`-c` names. It refuses to overwrite an
existing file unless you pass `--force`.

```bash
backup-git-repos config init
```

```yaml
dest: /srv/backups/git
forges:
  - name: work # becomes the top-level folder for this forge's repos
    kind: gitlab
    url: https://gitlab.example.com
    token_env: WORK_GITLAB_TOKEN
  - name: home
    kind: forgejo
    url: https://git.example.org
    token_env: HOME_FORGEJO_TOKEN
  - name: personal
    kind: github
    token_env: PERSONAL_GITHUB_TOKEN
```

```bash
export WORK_GITLAB_TOKEN=glpat-...
export HOME_FORGEJO_TOKEN=...
export PERSONAL_GITHUB_TOKEN=ghp_...
```

`url` is only for a self-hosted forge; GitHub.com is the one instance, so a
`github` entry never sets it. Its token needs the classic `repo` scope, or a
fine-grained token with read access to contents and metadata on every
repository you want backed up.

A forge entry can set `token` instead of `token_env`: the literal token,
right in the file, if you'd rather not manage a matching environment
variable for it. Setting both on the same entry is a config error, not a
silent pick-one. `token_env` stays the recommended form — whatever backs up
this tool's own backup tree (a git remote, a sync job, a snapshot) picks up
a `token` in the file the same as any other line in it, so treat a config
carrying one as a secret: `chmod 600` it, and don't check it in.

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
```

### Restoring a repository

A mirror is a normal bare repository, so cloning out of it is the whole
restore:

```bash
git clone /srv/backups/git/home/team/repo.git restored-repo
```

From an archive, extract first:

```bash
tar xzf /srv/backups/git/archive/home/team/repo.tar.gz
git clone repo.git restored-repo
```

### Flags

- `--config, -c`: path to the YAML config (required)
- `--dest, -d`: override the destination directory from the config. Required
  on `run` unless the config sets `dest`. A leading `~` (in this flag, in
  `dest` in the config file, or in `--archive-dir`) expands to your home
  directory
- `--forge`: repeatable; restrict the run to named forges
- `--state`: `all` \| `active` \| `archived` — which repositories to mirror
  (default `all`)
- `--archive`: `none` \| `all` \| `active` \| `archived` — which repositories
  also get written out as a `.tar.gz` (default `none`)
- `--archive-dir`: where archives go (default `<dest>/archive`)
- `--concurrency, -j`: repositories mirrored in parallel (default the number
  of CPUs)
- `--timeout`: per-repository timeout (default `30m`)

`backup-git-repos list` runs the same discovery and filtering without cloning
anything, which is the fast way to check a config is picking up what you
expect.

`backup-git-repos config init [--force]` writes the starter config described
under [Configuration](#configuration).

## License

This project is licensed under the GNU General Public License v3.0 — see the
[LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome. Open a pull request.

Read [CONTRIBUTING.md](CONTRIBUTING.md) first. It covers the setup, how the
test suites are split, and what each linter is for.
