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
- Optionally exports a repository's issues, its releases and their uploaded
  assets, or its pull/merge requests and their review comments, alongside
  its mirror (`--export-metadata issues,releases,pull-requests`) --
  metadata a bare git mirror never captures on its own, since none of it
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

Or, on Nix or NixOS: `nix run github:alrayyes/backup-git-repos` to try it, or
`nix profile install github:alrayyes/backup-git-repos` to keep it. No hosted
binary cache -- a first run/install builds from source, same tradeoff as
every other from-source path here.

Or run the container image, `git` and all — a natural fit for a scheduled job.
It runs as UID/GID `1000`, not root, so the destination directory needs to be
writable by that UID on the host:

```bash
mkdir -p /srv/backups/git && chown 1000:1000 /srv/backups/git
docker run --rm \
  --read-only --tmpfs /tmp \
  --cap-drop=ALL --security-opt=no-new-privileges \
  --memory=512m --cpus=2 \
  -v /srv/backups/git:/srv/backups/git -v ./config.yaml:/config.yaml:ro \
  ghcr.io/alrayyes/backup-git-repos:latest run --config /config.yaml
```

The image needs no capability beyond the default network access every
container gets, so `--cap-drop=ALL` alone is the whole capability line, with
nothing to add back. `--tmpfs /tmp` matters specifically for `--archive`: an
already-archived repository mirrors into a scratch directory under `/tmp`
before being written out as a `.tar.gz` (see [Usage](#usage)), which needs
somewhere writable once `--read-only` locks the rest of the image's own
filesystem. Size the mount for your largest archived repository if the
default (half the host's RAM) isn't enough, and size `--memory`/`--cpus`
for how many repositories `--concurrency` mirrors at once.

Images are multi-arch (`linux/amd64`, `linux/arm64`), tagged `latest` and per
version, and published alongside every release at
[ghcr.io/alrayyes/backup-git-repos](https://github.com/alrayyes/backup-git-repos/pkgs/container/backup-git-repos).

## Configuration

List every forge in a YAML file, the token included. That's the default
and recommended form: paste the token straight into `token`, and there's
nothing else to wire up before `run` works. Treat a config carrying one as
a secret, the same care you'd give any file with credentials in it —
`chmod 600` it, and don't check it in. If you'd rather keep the token out
of the file entirely, `token_env` is the opt-in for that; see below.

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
`github` entry never sets it.

Each kind's token needs enough to list and clone every repository you want
backed up, and nothing more:

- **GitLab**: a personal access token with the `read_api` and
  `read_repository` scopes. `read_api` covers listing projects;
  `read_repository` covers the git clone itself.
- **Forgejo**: a token with the `read:repository` scope (fine-grained
  tokens) or, on a version old enough to only offer full-access tokens, one
  scoped as narrowly as your Forgejo instance allows.
- **GitHub**: the classic `repo` scope, or a fine-grained token with read
  access to contents and metadata on every repository you want backed up.
  For a classic token, `run` and `list` check the scopes GitHub reports
  back and fail fast if `repo` is missing, rather than silently backing up
  only the public repositories. GitHub gives no equivalent signal for a
  fine-grained token, so that check can't run against one -- double-check a
  fine-grained token's repository access is set to what you expect.

A `forgejo` entry can also set `skip_mirrors: true` to exclude repositories
Forgejo itself reports as mirrors of an external upstream from both
listing and mirroring — there's no point re-backing-up content that
already lives at its real source elsewhere. Off by default; other kinds
ignore the field. Every repository it excludes this way is logged by name,
so a shorter backup than expected has an answer.

A forge entry can set `token_env` instead of `token`: the name of an
environment variable holding the token, if you'd rather keep it out of the
file — the tool reads the variable at startup and fails before touching
the network if it's unset. Setting both on the same entry is a config
error, not a silent pick-one. Reach for it when whatever backs up this
tool's own backup tree (a git remote, a sync job, a snapshot) would
otherwise pick up a `token` in the file the same as any other line in it:

```yaml
forges:
  - name: work
    kind: gitlab
    url: https://gitlab.example.com
    token_env: WORK_GITLAB_TOKEN
```

```bash
export WORK_GITLAB_TOKEN=glpat-...
```

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

### Restoring a repository

A `.tar.gz` holds the same bare mirror as the `.git` directory next to it,
not a working-tree checkout -- extracting it alone doesn't hand you files
you can edit. That's deliberate: a bare mirror is the only form that keeps
every branch and tag, not just whichever one would've been checked out.
`git clone` is what turns either one into an ordinary working copy.

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

If the repository used Git LFS, the restoring machine needs `git-lfs`
installed too -- `git clone` checks out the working tree with plain pointer
files otherwise, and `git lfs pull` afterward is what turns them into the
real file contents, the same way it would against the original forge.

### Metadata export

A bare mirror captures every commit, branch and tag, but nothing else a
forge stores about a repository -- issues, pull/merge requests, releases,
CI/CD config -- because none of it lives in the git history. `--export-metadata`
is opt-in per kind, comma-separated or repeated: `--export-metadata
issues,releases,pull-requests`. Off by default, and off is exactly today's
behaviour -- nothing about a run changes unless you ask for a kind by
name. `issues`, `releases` and `pull-requests` are the kinds this release
supports; more (CI/CD config) are planned as a separate, later addition to
the same flag, opted into individually.

Each kind lands in its own subdirectory alongside the repository's mirror,
never inside it:

```text
/srv/backups/git/home/team/repo.git/           # the mirror, as always
/srv/backups/git/home/team/repo.metadata/issues/1.json
/srv/backups/git/home/team/repo.metadata/issues/2.json
/srv/backups/git/home/team/repo.metadata/releases/v1.2.0/release.json
/srv/backups/git/home/team/repo.metadata/releases/v1.2.0/assets/backup-git-repos_linux_amd64.tar.gz
/srv/backups/git/home/team/repo.metadata/pull-requests/7.json
```

One JSON file per issue, named by its number, holding its title, body,
author, state (`open` or `closed`), labels, timestamps, and every comment on
it -- an issue with no comments is still written, with an empty `"comments"`
list, not skipped:

```json
{
  "number": 2,
  "title": "some things need saying",
  "body": "...",
  "author": "alice",
  "state": "closed",
  "labels": ["bug"],
  "created_at": "2026-08-15T08:00:00Z",
  "updated_at": "2026-08-15T10:00:00Z",
  "closed_at": "2026-08-15T10:00:00Z",
  "comments": [{ "author": "bob", "body": "confirmed", "created_at": "2026-08-15T09:00:00Z" }]
}
```

What isn't exported: a private issue or comment your token can't itself
read comes back the same way it would from any other API call --
`backup-git-repos` exports only what the configured token can already see,
never more. A forge administrator's own view (private discussions restricted
to staff, say) isn't something a personal access token reaches either, so
those stay out of the backup along with everything else the token was
never granted.

On GitLab, only genuine issues are written -- merge requests live on their
own, entirely separate API endpoint, so there's nothing to filter out. On
Forgejo and GitHub, issues and pull requests share one API endpoint; the
exporter asks for issues only, so a pull request never shows up misfiled as
one.

`--export-metadata releases` writes each release's notes and every asset
someone uploaded to it -- never the source-code tarball/zipball a forge
generates automatically for every tag, since that's already recoverable
from the mirror itself. A release with no uploaded assets still gets its
`release.json`, and no `assets/` directory is created for it:

```text
/srv/backups/git/home/team/repo.metadata/releases/v1.2.0/release.json
/srv/backups/git/home/team/repo.metadata/releases/v1.2.0/assets/backup-git-repos_linux_amd64.tar.gz
```

```json
{
  "tag_name": "v1.2.0",
  "name": "v1.2.0",
  "body": "### Fixed\n\n- ...",
  "author": "alice",
  "created_at": "2026-08-15T08:00:00Z",
  "published_at": "2026-08-15T08:05:00Z",
  "assets": [{ "name": "backup-git-repos_linux_amd64.tar.gz", "size": 8388608 }]
}
```

A release's own tag is what names its directory (with `/` and `\`
replaced, so a tag like `releases/v1` doesn't add unintended nesting); two
releases on the same repository never collide on disk. A single asset is
capped at 4 GiB -- a runaway or interrupted download is removed rather than
left as a partial file that would read as complete, and the export of that
one asset fails while the rest of the release still lands. Assets are
fetched with whatever rate limit the forge's API already enforces; a
repository with many large releases can take noticeably longer to export
than one with none.

`pull-requests` follows the same shape, one JSON file per pull/merge
request, named by its number, holding its title, body, author, state
(`open`, `closed`, or `merged` -- a merged one is reported as `merged`
rather than `closed`, since that's exactly the distinction a restored
history needs to tell "abandoned" from "landed" apart), source branch,
target branch, timestamps, and its review comments. A comment anchored to
a specific line of the diff carries `file` and `line`; a general review
comment that isn't diff-anchored carries neither:

```json
{
  "number": 7,
  "title": "add session refresh endpoint",
  "body": "...",
  "author": "alice",
  "state": "merged",
  "source_branch": "feat/session-refresh",
  "target_branch": "main",
  "created_at": "2026-08-15T08:00:00Z",
  "updated_at": "2026-08-15T10:00:00Z",
  "merged_at": "2026-08-15T10:00:00Z",
  "comments": [
    { "author": "bob", "body": "looks good", "created_at": "2026-08-15T09:00:00Z" },
    {
      "author": "bob",
      "body": "off by one here",
      "created_at": "2026-08-15T09:05:00Z",
      "file": "auth.go",
      "line": 42
    }
  ]
}
```

The same exclusions as `issues` apply: only what the configured token can
already see gets written, never more.

### Flags

Every `run`/`list` flag below except `--config` also has an environment
variable form, named `BACKUP_GIT_REPOS_<FLAG_NAME>` -- the flag's own name,
upper-cased with dashes replaced by underscores, so `--archive-dir` becomes
`BACKUP_GIT_REPOS_ARCHIVE_DIR`. A flag wins over its environment variable,
which wins over the config file's own value for the same setting (`dest`
is the one the config file has a name for today), which falls back to the
flag's own built-in default -- the standard flag > environment variable >
config file > default precedence, useful for a container or CI job with no
file to mount, without giving up the config file for everything else.
`token`/`token_env` on a forge entry keep their own resolution,
unaffected by this.

- `--config, -c`: path to the YAML config (default:
  `$XDG_CONFIG_HOME/backup-git-repos/config.yaml`, falling back to
  `~/.config/backup-git-repos/config.yaml`; required if no file exists at
  that path). No environment-variable form -- it names the file this
  precedence itself reads from
- `--dest, -d` (`BACKUP_GIT_REPOS_DEST`): override the destination
  directory from the config. Required on `run` unless the config sets
  `dest`. A leading `~` (in this flag, in `dest` in the config file, or in
  `--archive-dir`) expands to your home directory
- `--forge` (`BACKUP_GIT_REPOS_FORGE`): repeatable; restrict the run to
  named forges
- `--state` (`BACKUP_GIT_REPOS_STATE`): `all` \| `active` \| `archived` —
  which repositories to mirror (default `all`)
- `--archive` (`BACKUP_GIT_REPOS_ARCHIVE`): `none` \| `all` \| `active` \|
  `archived` — which repositories also get written out as a `.tar.gz`
  (default `none`)
- `--archive-dir` (`BACKUP_GIT_REPOS_ARCHIVE_DIR`): where archives go
  (default `<dest>/archive`)
- `--export-metadata` (`BACKUP_GIT_REPOS_EXPORT_METADATA`): comma-separated
  or repeated; also export these kinds of forge metadata alongside each
  repository's mirror, under `<dest>/<repo>.metadata/<kind>/` (see
  [Metadata export](#metadata-export)). Currently, `issues`, `releases`
  and `pull-requests`. Off by default: a run's behaviour is unchanged from
  before this flag existed unless you name a kind
- `--concurrency, -j` (`BACKUP_GIT_REPOS_CONCURRENCY`): repositories
  mirrored in parallel (default the number of CPUs)
- `--timeout` (`BACKUP_GIT_REPOS_TIMEOUT`): per-repository timeout
  (default `30m`)
- `--verbose, -v` (`BACKUP_GIT_REPOS_VERBOSE`): log each repository as it
  starts mirroring and archiving, not just failures and the final summary
- `--dry-run` (`BACKUP_GIT_REPOS_DRY_RUN`): print what the run would do --
  `clone` or `update` per repository, plus `archive` where `--archive`
  selects it, and `prune` where `--prune-removed` would delete a mirror --
  without touching git or writing anything. Still needs `--dest`: telling
  clone from update means checking what's already on disk
- `--prune-removed` (`BACKUP_GIT_REPOS_PRUNE_REMOVED`): **deletes** a
  mirror (and its `.tar.gz`, if archived) once its repository no longer
  appears on the forge at all. Off by default: a run only ever adds or
  refreshes mirrors otherwise, and a stale one -- left behind by a
  repository deleted or renamed upstream -- is only warned about by name,
  never touched. Staleness is always judged against every repository the
  forge reports, not just this run's own `--state` selection, so pairing
  this with `--state active` or `--state archived` never deletes a mirror
  that's merely excluded by that filter

`run` also prints a live `<forge>: done/total` progress line to stderr while
it works, redrawn in place — only when stderr is a terminal, since a
carriage-return-redrawn line is unreadable once piped or redirected into a
file or CI log.

`backup-git-repos list` runs the same discovery and filtering without cloning
anything, which is the fast way to check a config is picking up what you
expect. `run --dry-run` goes a step further: same idea, but it also says
what each repository would do once `--dest` is factored in.

`backup-git-repos config init [--force]` writes the starter config described
under [Configuration](#configuration).

## License

This project is licensed under the GNU General Public License v3.0 — see the
[LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome. Open a pull request.

Read [CONTRIBUTING.md](CONTRIBUTING.md) first. It covers the setup, how the
test suites are split, and what each linter is for.
