## Why

A bare mirror only captures the tag a release points at — it never captures the release notes or the binaries someone uploaded to it. When a forge goes away permanently, that content goes with it even though the code survives. Issue export (#81) already proved the opt-in `--export-metadata` pattern; this extends it to releases, split out of the umbrella ticket #7 into its own scope (#83).

## What changes

- New `MetadataReleases` kind for `--export-metadata`, alongside the existing `issues` kind.
- New `Release` / `ReleaseAsset` types and `WriteRelease` / `WriteReleaseAsset` helpers in the root package, following the same shared-writer shape `WriteIssue` already established for issues.
- One `ReleaseExporter` per forge adapter (`internal/forgejo`, `internal/github`, `internal/gitlab`), each implementing `MetadataExporter`, paging that forge's releases API and downloading every uploaded asset — never the forge-generated source archive, which each forge already reports separately from uploaded assets.
- A hard 4 GiB cap per asset (`MaxReleaseAssetSize`), enforced while streaming so a runaway asset can't balloon disk usage past what a caller expects from one repository's backup.
- `cmd/backup-git-repos/main.go` wires a `ReleaseExporter` into all three forges' `newRunner`, alongside the existing issue exporter.

## Capabilities

### New capabilities

- `metadata-export/releases`: exporting a repository's releases (notes, author, timestamps) and their uploaded assets to disk, opt-in via `--export-metadata releases`.

### Modified capabilities

(none — release export is additive; the existing issues export capability is unchanged)

## Impact

- `metadata.go`: new `MetadataReleases` constant, `Release`/`ReleaseAsset` types, `WriteRelease`/`WriteReleaseAsset`/`releaseDir`/`sanitizeName`, `MaxReleaseAssetSize`, `ErrReleaseAssetTooLarge`.
- `internal/forgejo/release.go`, `internal/github/release.go`, `internal/gitlab/release.go`: new `ReleaseExporter` per forge, plus their tests.
- `cli.go`, `cmd/backup-git-repos/main.go`: `--export-metadata` help text and `newRunner` wiring.
- **Known gap:** the README hasn't been updated. It still states issues is the only metadata kind this release supports, and documents neither the releases on-disk format, the 4 GiB asset cap, nor rate-limit caveats — the ticket's Definition of Done requires this. Tracked as an open task in tasks.md.
