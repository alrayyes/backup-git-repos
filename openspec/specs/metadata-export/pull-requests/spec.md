# `metadata-export/pull-requests` specification

## Purpose

Defines how backup-git-repos exports a repository's pull and merge
requests, including review comments, as an opt-in metadata kind alongside
the git mirror.

## Requirements

### Requirement: pull/merge request export is opt-in

The system SHALL export a repository's pull and merge requests only when
`--export-metadata` includes `pull-requests`. When it is not included,
behavior SHALL be unchanged from before this capability existed.

#### Scenario: export not requested

- **WHEN** a repository is backed up and `--export-metadata` does not
  include `pull-requests`
- **THEN** no pull/merge request data is fetched or written

#### Scenario: export requested

- **WHEN** a repository is backed up and `--export-metadata` includes
  `pull-requests`
- **THEN** every pull/merge request on the repository, open and
  closed/merged, is written to
  `<dest>/<repo.Path>.metadata/pull-requests/<number>.json`

### Requirement: exported pull/merge request content

Each exported pull/merge request SHALL include its number, title, body,
author, state, source branch, target branch, creation timestamp, last
update timestamp, close timestamp (if closed), and merge timestamp (if
merged).

#### Scenario: open pull request

- **WHEN** a pull/merge request is open
- **THEN** its exported record's close and merge timestamps are absent

#### Scenario: merged pull request

- **WHEN** a pull/merge request has been merged
- **THEN** its exported record includes the timestamp it was merged at

#### Scenario: closed without merging

- **WHEN** a pull/merge request was closed without being merged
- **THEN** its exported record includes a close timestamp and no merge
  timestamp

### Requirement: review comments are preserved (general and inline)

Each exported pull/merge request SHALL include every comment on it, both
general (thread-level) comments and inline (diff-anchored) review comments.
An inline comment's record SHALL preserve the path and line it anchors to;
a general comment's record SHALL NOT carry a path or line.

#### Scenario: general comment

- **WHEN** a pull/merge request has a comment posted on the overall
  discussion thread, not anchored to a specific line
- **THEN** the exported comment includes its author, body, and creation
  timestamp, with neither a path nor a line

#### Scenario: inline review comment

- **WHEN** a pull/merge request has a comment anchored to a specific file
  and line in the diff
- **THEN** the exported comment includes its author, body, creation
  timestamp, path, and line number

#### Scenario: no comments

- **WHEN** a pull/merge request has no comments
- **THEN** its exported record's comments list is present and empty, not
  omitted or null

### Requirement: pagination completeness

The system SHALL retrieve every pull/merge request and every comment on
each one, across all pages the forge's API returns, rather than only the
first page.

#### Scenario: more pull requests than fit in one API page

- **WHEN** a repository has more pull/merge requests than one page of the
  forge's list API returns
- **THEN** every one of them is exported, not just the first page

#### Scenario: more comments than fit in one API page

- **WHEN** a pull/merge request has more comments than one page of the
  forge's comments API returns
- **THEN** every one of them is exported, not just the first page
