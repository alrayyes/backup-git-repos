# Contributing

How to work on the tool. If you only want to run it, the [README](README.md)
has everything you need and none of this.

The short version: the failing test comes first, the linters run in git hooks
and in CI on the same commands, and every one of them is here for a reason
recorded below.

## What you need

Running the tool needs Go and `git` on `PATH`. Working on it needs:

- **Go 1.26** or newer, and **golangci-lint 2.x** — the linter and the fixer
  both.
- **[bun](https://bun.sh)** for the Node-shaped tooling: commitlint, Biome,
  Prettier, markdownlint-cli2 and lefthook. Not npm. The lockfile is
  `bun.lock`.
- **Docker**, for the integration test suites (`go test ./...` never
  touches it; `-tags=integration` does) and for linting `Dockerfile` with
  hadolint, which runs from its own image rather than a local install.
- **[Vale](https://vale.sh)**, optional. The hooks skip it when it isn't on
  your `PATH`, and CI runs it either way.

## Getting set up

```bash
# Install the Go tooling
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Installs the Node-shaped tooling and the git hooks: lefthook is pinned in
# package.json, and the `prepare` script runs it for you.
bun install

# Run the fast suite
go test ./...

# Run and fix by hand
golangci-lint run
golangci-lint fmt

# Test commit message format
echo "feat: add new feature" | bunx commitlint
```

## Tests

Three lanes, because a real GitLab instance is too heavy to boot on every
push.

- `go test ./...` — unit tests and a contract suite run against an in-memory
  fake. No network, no containers. This is what `pre-push` runs.
- `go test -tags=integration -race ./...` — boots a real Forgejo container
  (seconds, via the official `testcontainers-go` module) and exercises the
  full mirror-and-refresh path against it. The GitLab and GitHub halves of
  this lane are served from recorded API fixtures in `testdata/`, not a live
  GitLab or GitHub.com. CI runs this on every pull request.
- `go test -tags='integration gitlab' -run GitLab ./...` — boots a real
  GitLab CE container, which wants several minutes and several gigabytes. CI
  runs this nightly and on `workflow_dispatch`, never on a pull request.

GitHub.com has no self-hosted equivalent to boot, so its recorded fixtures in
`internal/github/testdata/` are hand-authored against GitHub's REST API
documentation rather than captured from a live instance the way GitLab's
`-update` flag does. Keep them in sync by hand if that API's response shape
changes.

The contract suite lives once, in `testing.go`, and runs against the fake,
the Forgejo adapter, the GitLab adapter, and the GitHub adapter alike — the
thing that keeps a handwritten fake honest is a real dependency checked
against the same assertions. When the GitLab fixtures drift from what the
live API actually returns, the nightly run's `-update` flag refreshes them
and the diff shows what changed.

Acceptance tests keep the specification separate from the driver: what the
tool does, in domain terms, doesn't know whether it's talking to a fake or a
container. Only the driver does.

## Git hooks

This project uses [lefthook](https://github.com/evilmartians/lefthook) to
manage git hooks, pinned in `package.json` like every other tool here, so `bun
install` puts them in place and everyone gets the same version:

- **pre-commit**: fixes staged files in place — `golangci-lint fmt` on Go,
  `prettier --write` then `markdownlint-cli2 --fix` on Markdown, `prettier
--write` on YAML, `biome check --write` on JSON. `Dockerfile` gets
  [hadolint](https://github.com/hadolint/hadolint) instead, which has no
  fixer, so it checks and fails rather than rewriting anything. It needs
  Docker, same as the integration suites below.
- **commit-msg**: validates commit messages with
  [commitlint](https://commitlint.js.org/) against [Conventional
  Commits](https://www.conventionalcommits.org/).
- **pre-push**: runs `golangci-lint run`, `go test -race ./...`, then
  re-checks every preceding linter across the whole repository, so nothing
  reaches the remote that CI would reject.

The container suites are the deliberate omission from `pre-push`. A Forgejo
boot is more than a push should wait on, and CI runs it on every pull request
regardless. Run `go test -tags=integration ./...` yourself before pushing a
change to an adapter.

The hooks and the GitHub Actions workflows run the same commands on purpose.
The hook catches a problem early; CI is the gate you can't skip.

## Prose

[Vale](https://vale.sh) checks style: house voice, weasel words, corporate
speak. It uses the Google and proselint packages, which `vale sync` downloads
rather than the repo committing them, so install Vale and run `vale sync`
once before `bun run lint:prose` works. The hooks run Vale when it's on
`PATH` and quietly skip it otherwise; CI runs it either way and only warns,
because a merge blocked by an opinion teaches people to reach for
`--no-verify`.

[ltex-cli-plus](https://github.com/ltex-plus/ltex-ls-plus) checks mechanics:
grammar, spelling, punctuation, by wrapping LanguageTool. This one fails the
build, because mechanics have a right answer. It stays out of the git hooks,
since it's a ~300 MB download shipping its own Java runtime, so run the same
engine in your editor over LSP (`ltex-ls-plus`, or `harper-ls` if you want
something lighter) and let CI be the fallback rather than the first time you
hear about a typo.

`styles/House/` holds the rules no published style guide covers:
`Filler.yml` for vocabulary that says nothing, `EmDash.yml` for a paragraph
leaning on more than one em-dash where a full stop would do.

Project vocabulary lives in two places, one per tool:
`styles/config/vocabularies/House/accept.txt` for Vale, `ltex.dictionary` in
`.ltex.json` for LTeX. Add new jargon, such as a forge name or a flag, to
both. Vale's copy also pins the casing, since `Vale.Terms` is on: spell a
product name any other way and it says so.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/),
checked by commitlint on `commit-msg`:

```text
<type>[optional scope]: <description>

Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
```

Examples:

```text
feat: list forgejo repos over the api
fix(mirror): keep the token out of .git/config
docs: document restoring from an archive
```

Configuration is in `.commitlintrc.json` and follows
`@commitlint/config-conventional`.

## Releases

[release-please](https://github.com/googleapis/release-please) reads the
Conventional Commits on `main` and keeps a release pull request open with the
next version and changelog. Merging it tags the release, which
[goreleaser](https://goreleaser.com/) then builds and attaches binaries to, as
a second job in the same workflow gated on the tag having just been created.
Nobody picks a version by hand.

That same goreleaser job also builds and pushes a multi-arch (`linux/amd64`,
`linux/arm64`) container image to `ghcr.io/alrayyes/backup-git-repos`, tagged
`latest` and with the version. `Dockerfile` is a plain runtime image, not a
build stage -- goreleaser cross-compiles the binaries first and stages each
platform's into the image build context itself
(`dockers_v2` in `.goreleaser.yaml`).
