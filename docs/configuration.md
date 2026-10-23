# Configuration reference

The basics (`config init`, a minimal file) are in the
[README](../README.md#configuration). This page covers tokens and the per-forge options.

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

A forge entry can also set `token_command`: a shell command whose trimmed
stdout becomes the token, for pulling it straight from `pass`, `hush-hush`,
an age-encrypted file, or a keyring CLI instead of writing it anywhere the
config file or the environment would otherwise hold it. It wins over
`token`/`token_env` on the same entry unconditionally -- setting it is
itself the decision to prefer it -- and a non-zero exit fails the run
rather than backing up with an empty credential:

```yaml
forges:
  - name: work
    kind: gitlab
    url: https://gitlab.example.com
    token_command: pass show work/gitlab-token
```
