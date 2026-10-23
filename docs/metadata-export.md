# Metadata export

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
