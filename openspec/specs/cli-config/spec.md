# `cli-config` specification

## Purpose

Defines how backup-git-repos resolves each CLI option's value across
flag, environment variable, config file, and built-in default, and the
precedence between those sources.

## Requirements

### Requirement: layered option resolution

Every `run`/`list` option that has a command-line flag SHALL also be settable by an environment variable and, where the config file already carries a matching value, by the config file. A value from a higher-precedence source SHALL be used in preference to a lower one; a source that supplies no value SHALL fall through to the next.

#### Scenario: only the environment variable is set

- **WHEN** `BACKUP_GIT_REPOS_DEST` is set and neither `--dest` nor the config file's `dest` is set
- **THEN** the value from `BACKUP_GIT_REPOS_DEST` is used as the backup destination

#### Scenario: flag overrides the environment variable

- **WHEN** both `--dest` and `BACKUP_GIT_REPOS_DEST` are set to different values
- **THEN** the value from `--dest` is used

#### Scenario: environment variable overrides the config file

- **WHEN** both `BACKUP_GIT_REPOS_DEST` and the config file's `dest` are set to different values, and `--dest` is not passed
- **THEN** the value from `BACKUP_GIT_REPOS_DEST` is used

#### Scenario: nothing set falls back to the built-in default

- **WHEN** an option's flag, environment variable, and config-file value are all unset
- **THEN** the option's built-in default is used, unchanged from today's behavior

### Requirement: environment variable naming

Each `run`/`list` flag's environment-variable form SHALL be named `BACKUP_GIT_REPOS_<FLAG_NAME>`, with the flag's name upper-cased and its dashes replaced by underscores (for example, `--archive-dir` becomes `BACKUP_GIT_REPOS_ARCHIVE_DIR`).

#### Scenario: multi-word flag maps predictably

- **WHEN** `BACKUP_GIT_REPOS_ARCHIVE_DIR` is set and `--archive-dir` is not passed
- **THEN** it is used as the archive directory, following the same naming rule as every other option

### Requirement: forge credentials keep their own resolution

A forge's credential (`token` / `token_env` in the config file) SHALL continue to resolve exactly as it does today, independent of the general flag/environment/config-file layering this capability defines for run options.

#### Scenario: `token_env` is unaffected by the new layering

- **WHEN** a forge entry sets `token_env` and the named environment variable is set
- **THEN** the forge's token resolves from that variable, the same as before this capability existed
