# backup-git-repos

[![Test](https://github.com/alrayyes/backup-git-repos/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/test.yml)
[![Lint](https://github.com/alrayyes/backup-git-repos/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/lint.yml)
[![Prose](https://github.com/alrayyes/backup-git-repos/actions/workflows/prose.yml/badge.svg?branch=main)](https://github.com/alrayyes/backup-git-repos/actions/workflows/prose.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/alrayyes/backup-git-repos.svg)](https://pkg.go.dev/github.com/alrayyes/backup-git-repos)
[![Release](https://img.shields.io/github/v/release/alrayyes/backup-git-repos)](https://github.com/alrayyes/backup-git-repos/releases)
[![Licence](https://img.shields.io/github/license/alrayyes/backup-git-repos)](LICENSE)

Backs up every repository on a self-hosted GitLab or Forgejo instance to a
local, restorable copy.

A backup only counts if it survives the forge going away, so what
`backup-git-repos` keeps is a bare mirror clone of each repository: every
branch, every tag, and the same namespace folder structure the forge used. It
tells archived repositories from active ones, and can back up either set, both,
or write either out as a `.tar.gz` alongside the mirror.

## Status

This is an early, in-progress build. Mirroring works end to end for both
Forgejo and GitLab. The `.tar.gz` archive option hasn't landed yet, so
`--archive` is still design rather than something you can run. This README is
updated as each piece ships.

## Features

- Mirrors every branch, tag and ref, not just the default branch
- Keeps the forge's own namespace structure on disk
- Filters by archived, active, or all repositories
- Refreshes existing mirrors incrementally instead of re-cloning
- Optionally writes archived, active, or all repositories out as `.tar.gz`
- Backs up several forges in one run: GitLab instances, Forgejo instances, or
  a mix of both

## Requirements

- **Go 1.26** or newer to build it.
- **git**, on `PATH`, to do the actual cloning. The tool shells out to it
  rather than re-implementing the protocol, which is what makes an incremental
  mirror refresh fast and the resulting `.git` directory exactly what `git
clone` produces anywhere else.
- A personal access token for each forge you back up, with read access to
  every repository you want. See [Configuration](#configuration) for where it
  goes — never in the config file itself.
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

List every forge in a YAML file. A token never goes in the file, only the
name of the environment variable holding it — the tool reads the variable at
startup and fails before touching the network if it's unset.

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
```

```bash
export WORK_GITLAB_TOKEN=glpat-...
export HOME_FORGEJO_TOKEN=...
```

## Usage

```bash
backup-git-repos run --config ./config.yaml
```

Resulting layout:

```text
/srv/backups/git/
  work/group/subgroup/repo.git/           # bare mirror
  home/team/repo.git/
  archive/work/group/subgroup/repo.tar.gz # only when --archive is set
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

- `--config, -c`: path to the YAML config (default
  `$XDG_CONFIG_HOME/backup-git-repos/config.yaml`)
- `--dest, -d`: override the destination directory from the config
- `--forge`: repeatable; restrict the run to named forges
- `--state`: `all` \| `active` \| `archived` — which repositories to mirror
  (default `all`)
- `--archive`: `none` \| `all` \| `active` \| `archived` — which repositories
  also get written out as a `.tar.gz` (default `none`)
- `--archive-dir`: where archives go (default `<dest>/archive`)
- `--concurrency, -j`: repositories mirrored in parallel (default the number
  of CPUs)
- `--timeout`: per-repository timeout (default `30m`)
- `--dry-run`: print what would happen, clone nothing
- `--log-level`, `--log-format`: `debug`\|`info`\|`warn`\|`error`,
  `text`\|`json`

`backup-git-repos list` runs the same discovery and filtering without cloning
anything, which is the fast way to check a config is picking up what you
expect.

## License

This project is licensed under the GNU General Public License v3.0 — see the
[LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome. Open a pull request.

Read [CONTRIBUTING.md](CONTRIBUTING.md) first. It covers the setup, how the
test suites are split, and what each linter is for.
