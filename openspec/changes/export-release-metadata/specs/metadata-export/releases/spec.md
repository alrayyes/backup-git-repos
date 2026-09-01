## Purpose

Preserves a repository's releases, including their notes and any uploaded assets, so a release's binaries and changelog survive a forge going away — a bare git mirror only captures the tag a release points at.

<!-- vale Google.Headings = NO -->

## ADDED Requirements

<!-- vale Google.Headings = YES -->

### Requirement: releases export is opt-in via --export-metadata

The system SHALL export a repository's releases only when `releases` is one of the values passed to `--export-metadata`. When it is not selected, backup behavior SHALL be unchanged from a run with no metadata export at all.

#### Scenario: metadata export not enabled

- **WHEN** a repository is backed up with `--export-metadata` unset, or set without `releases`
- **THEN** no release notes or release assets are written, and every other aspect of the backup is unchanged from before this capability existed

#### Scenario: releases export enabled

- **WHEN** a repository is backed up with `--export-metadata releases` (alone or comma-separated/repeated with other kinds)
- **THEN** every release on the repository is exported alongside the mirror

### Requirement: release notes and assets are written per release, collision-free

The system SHALL write each release's notes and every uploaded asset attached to it, under a location derived from the release's tag, such that two releases on the same repository never collide on disk.

#### Scenario: release with uploaded assets

- **WHEN** a repository has a release carrying one or more uploaded assets
- **THEN** the release's notes (tag, name, body, author, created and published timestamps) and every uploaded asset's content are saved to disk, addressable by the release's tag

#### Scenario: release with no uploaded assets

- **WHEN** a repository has a release with no uploaded assets (source archives only)
- **THEN** the release's notes are still saved, and no asset files or asset directory are created for that release

### Requirement: only uploaded assets are exported, not forge-generated source archives

The system SHALL export only assets a user uploaded to the release. It SHALL NOT export the source code archive (tarball/zipball) a forge generates automatically for every tagged release.

#### Scenario: release with only an auto-generated source archive

- **WHEN** a release has no user-uploaded assets, only the forge's own auto-generated source archive
- **THEN** the exported release has an empty asset list — the auto-generated archive is not downloaded or written

### Requirement: a single oversize asset does not consume unbounded disk space

The system SHALL cap how much of a single release asset it writes to disk, and SHALL leave no partial file behind when that cap is exceeded or the download otherwise fails.

#### Scenario: asset exceeds the size cap

- **WHEN** a release asset's content exceeds the system's maximum allowed asset size
- **THEN** the export of that asset fails, and no partial file for that asset is left on disk

#### Scenario: asset download fails partway

- **WHEN** a release asset's download is interrupted or errors partway through
- **THEN** the export of that asset fails, and no partial file for that asset is left on disk
