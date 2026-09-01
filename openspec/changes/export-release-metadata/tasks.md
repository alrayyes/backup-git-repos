## Core release model and writers

- [x] 1.1 Add `MetadataReleases` kind, `Release`/`ReleaseAsset` types to metadata.go and verify `metadata_test.go` covers their JSON shape
- [x] 1.2 Add `WriteRelease`, `releaseDir`, `sanitizeName` and verify unit tests cover tag sanitization (`/`, `\`, `""`, `.`, `..`) and the "no assets" case writing `"assets": []`
- [x] 1.3 Add `WriteReleaseAsset` with `MaxReleaseAssetSize` (4 GiB) enforcement via `io.LimitReader` and verify unit tests cover the at-cap, over-cap, and interrupted-download cases all leave no partial file

## Per-forge release exporters

- [x] 2.1 Implement `internal/forgejo.ReleaseExporter` (paging, asset download via rebuilt base URL, `"token <t>"` auth) and verify `release_test.go` / `release_unit_test.go` pass
- [x] 2.2 Implement `internal/github.ReleaseExporter` (paging, asset download via API URL + `Accept: application/octet-stream`, Bearer auth) and verify `release_test.go` / `release_unit_test.go` pass
- [x] 2.3 Implement `internal/gitlab.ReleaseExporter` (paging via `x-next-page`, `direct_asset_url`-or-`url` resolution, `PRIVATE-TOKEN` auth) and verify `release_test.go` / `release_unit_test.go` / `recorded_release_test.go` pass

## Wiring

- [x] 3.1 Register `NewReleaseExporter` alongside the issue exporter in all three forges' `newRunner` in cmd/backup-git-repos/main.go
- [x] 3.2 Update `--export-metadata`'s help text in `cli.go` to list `issues, releases` and verify `go build ./...` succeeds

## Documentation

- [ ] 4.1 Document `--export-metadata releases` in `README.md`: the on-disk layout (`<dest>/<repo.Path>.metadata/releases/<tag>/release.json` and `.../assets/<name>`), the 4 GiB per-asset cap, and any rate-limit caveats worth knowing before enabling it — verify by re-reading the README's metadata-export section end to end and confirming it no longer says issues is the only supported kind
