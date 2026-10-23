# Flags

Every `run`/`list` flag below except `--config` also has an environment
variable form, named `BACKUP_GIT_REPOS_<FLAG_NAME>` -- the flag's own name,
upper-cased with dashes replaced by underscores, so `--archive-dir` becomes
`BACKUP_GIT_REPOS_ARCHIVE_DIR`. A flag wins over its environment variable,
which wins over the config file's own value for the same setting (`dest`
is the one the config file has a name for today), which falls back to the
flag's own built-in default -- the standard flag > environment variable >
config file > default precedence. That's useful for a container or CI job
with no file to mount, without giving up the config file for everything else.
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
  [Metadata export](metadata-export.md)). Currently, `issues`, `releases`
  and `pull-requests`. Off by default: a run's behaviour is unchanged from
  before this flag existed unless you name a kind
- `--concurrency, -j` (`BACKUP_GIT_REPOS_CONCURRENCY`): repositories
  mirrored in parallel (default the number of CPUs)
- `--timeout` (`BACKUP_GIT_REPOS_TIMEOUT`): per-repository timeout
  (default `30m`)
- `--verbose, -v` (`BACKUP_GIT_REPOS_VERBOSE`): log each repository as it
  starts mirroring and archiving rather than only logging failures and the
  final summary
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
under [Configuration](../README.md#configuration).
