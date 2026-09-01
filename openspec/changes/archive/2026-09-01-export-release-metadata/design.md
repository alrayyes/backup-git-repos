## Context

See proposal.md - Why. Issue export (#81) already established the pattern this follows: a `MetadataExporter` interface (`Kind() MetadataKind`, `Export(ctx, repo, dir) error`), one implementation per forge adapter, gated per-run by `wantsMetadata(opts.ExportMetadata, exp.Kind())` in `run.go`'s `exportMetadata`. This design reuses that shape rather than inventing a parallel one.

## Goals / non-goals

**Goals:**

- Reuse the existing `MetadataExporter` contract and on-disk convention (`<dest>/<repo.Path>.metadata/<kind>/`) rather than a bespoke mechanism for releases.
- Keep asset downloads bounded and safe to interrupt: no partial files, no unbounded disk growth from one runaway asset.
- Keep the three forge adapters symmetric in shape even where their APIs differ (asset URL resolution, auth header).

**Non-Goals:**

- Rate-limit handling beyond what the existing HTTP client already does for issues — not addressed here even though the ticket's Definition of Done mentions documenting rate-limit caveats (a documentation gap, not a design gap; see Risks below).
- Resuming a partially downloaded asset across runs. A failed asset download is retried in full on the next run, not resumed.

## Decisions

**One `release.json` per release, written only after every asset for that release downloaded successfully.** Alternative considered: write `release.json` first, then assets. Rejected — a partial asset failure would leave a `release.json` on disk claiming an asset that isn't actually there, which is worse than the release being retried whole on the next run.

**Assets directory created lazily, only when a release actually has an asset to write.** Satisfies the ticket's "no empty asset files" acceptance criterion directly: `WriteReleaseAsset` is simply never called for an asset-less release, so there's nothing to special-case.

**Tag name, not release ID, is the on-disk directory key (`sanitizeName(tag)`).** Matches how a human would look for a release's backup (`git tag`), and is stable across forges that don't share an ID scheme. `/` and `\` in a tag (a legitimate tag like `releases/v1`) are replaced rather than allowed to add unintended nesting; `""`, `.`, `..` collapse to a placeholder rather than resolving to the parent directory or escaping it.

**4 GiB per-asset cap (`MaxReleaseAssetSize`), enforced by streaming through `io.LimitReader(body, cap+1)` and checking the byte count after.** Reading one byte past the cap distinguishes "exactly at the cap" from "over the cap" without buffering the whole asset in memory first. On overflow or any copy error, the partial file is removed rather than left looking complete.

**Per-forge asset URL handling differs, each for a documented reason, rather than being forced into one shared code path:**

- Forgejo and GitLab rebuild the forge-reported asset URL against the client's own configured base URL, keeping only its path along with its query string. Rationale: both forges' reported host/port come from their own configured external URL (`ROOT_URL`, GitLab's external URL), which inside a container is the container-internal address, not what the caller can actually reach — the same problem `Client.Remote` already solves for a repository's own clone URL.
- GitHub fetches through the API's own asset URL (not `browser_download_url`) with `Accept: application/octet-stream`, so the same authenticated request also works for a private repository.
- GitLab additionally falls back to an asset link's plain `url` when GitLab has set no `direct_asset_url` — a release link can point anywhere (an external URL someone manually attached), so it's used as-is rather than assumed to live on the GitLab instance.

**Exporters are always registered in `newRunner`, gated at run time by `Kind()`.** Matches how the issues exporter already works — `MetadataExporters` always includes every kind a forge supports, and `wantsMetadata` decides per run which of them actually execute. Keeps `newRunner` free of a conditional-registration branch per kind.

## Risks / trade-offs

- [A large release's asset download can be slow, and a slow one blocks the rest of that repository's export] → Bounded by the existing per-repository `--timeout`, same as every other network call in a run; not specific to releases.
- [The 4 GiB cap and rate-limit behavior aren't documented anywhere a user would see them before enabling `--export-metadata releases`] → README.md has not been updated (see proposal.md's Impact section). Tracked as an open task in tasks.md; out of scope for this design document to resolve since it's a documentation gap, not a technical one.
