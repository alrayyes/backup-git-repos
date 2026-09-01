## Why

Every run option except a forge's credential (`token_env`) can only be set via a flag or the config file today. Someone running this tool from a container or CI job with no file to mount has to generate a config file just to override, say, `--dest` or `--concurrency` once. `rules/cli.md`'s standard precedence for a CLI here (flags > environment variables > config file > built-in defaults) isn't met: the environment layer doesn't exist for anything but the credential.

## What changes

- Adopt `spf13/viper` to mediate configuration across three layers: cobra flags, environment variables, and the YAML config file, with flags binding to viper via `viper.BindPFlag`.
- Every `run`/`list` flag (`--dest`, `--state`, `--archive`, `--archive-dir`, `--concurrency`, `--timeout`, `--forge`, `--verbose`, `--dry-run`, `--prune-removed`, `--export-metadata`) gains an environment-variable form, prefixed `BACKUP_GIT_REPOS_` (for example `BACKUP_GIT_REPOS_DEST`).
- Precedence becomes flag > environment variable > config file value > built-in default, for every one of those options — generalizing the flag/config-file fallback `--dest` already has today.
- `LoadConfig`'s existing token/token_env resolution and validation (`ForgeConfig.Token`, `ErrAmbiguousToken`, `ErrMissingToken`) is unchanged: viper mediates those general run options, not a forge's credential, which keeps its own narrower environment-variable mechanism.
- README's Configuration section documents the environment-variable form for each option.

## Capabilities

### New capabilities

- `cli-config`: how `backup-git-repos run`/`list` resolve their options across flags, environment variables, and the config file, and in what order.

### Modified capabilities

(none — no existing capability spec covers CLI configuration yet)

## Impact

- `cli.go`: `cliFlags`, `NewRootCommand`, `addRunFlags`, `resolveDestPaths`, and related flag registration — flags need viper bindings instead of (or alongside) their current `cobra.Command.Flags()` defaults.
- `config.go`: `LoadConfig`/`Config` stay the source for forge entries and token resolution; viper's own config-file read needs to coexist with it rather than replace it, since `Config.Forges` (`token`, `token_env`, `skip_mirrors`) isn't flag-shaped.
- New dependency: `github.com/spf13/viper`, pinned to an exact version in `go.mod`.
- `README.md`'s Configuration section.
