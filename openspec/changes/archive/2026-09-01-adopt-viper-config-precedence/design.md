## Context

`cli.go` builds the command tree with `spf13/cobra` and stores flag values into a plain `cliFlags` struct via `cmd.Flags().StringVar(...)` etc. `config.go`'s `LoadConfig` separately reads the YAML config file into `Config`/`ForgeConfig`, resolving each forge's token from either a literal or `token_env`. `resolveDestPaths` is the one place today that already layers a flag over a config-file value (`--dest` over `cfg.Dest`), by hand. See proposal.md - Why for the gap this closes.

## Goals / non-goals

**Goals:**

- Every `run`/`list` flag gains an environment-variable form, with flag > environment variable > config file > default precedence, via `spf13/viper`.
- The existing config-file shape (`Config`, `ForgeConfig`, token/token_env resolution) is untouched.

**Non-Goals:**

- Changing how a forge's credential resolves (`token_env`, `ErrAmbiguousToken`, `ErrMissingToken`) — out of scope, see spec's "Forge credentials keep their own resolution."
- Adding new run options — this only adds an environment layer to options that already exist.
- Changing the config file's YAML schema.

## Decisions

**One viper instance per `run`/`list` invocation, built in `runBackup`.** The cobra flags bind to it with `viper.BindPFlag(name, cmd.Flags().Lookup(name))` right after `addRunFlags`/the command-specific flags are registered, and `viper.SetEnvPrefix("BACKUP_GIT_REPOS")` + `viper.AutomaticEnv()` supply the environment layer. `viper.SetConfigFile(configPath)` + `viper.ReadInConfig()` supplies the config-file layer for the same keys, reusing the config path `resolveConfigPath` already resolves. Alternative considered: hand-roll `os.LookupEnv` fallbacks per flag, matching `token_env`'s existing narrow pattern generalized — rejected because it's exactly the boilerplate viper exists to remove, and `rules/go.md` already names viper as the answer here.

**Every flag value is read back through viper (`viper.GetString`, `viper.GetInt`, `viper.GetDuration`, `viper.GetStringSlice`, `viper.GetBool`), not through `cliFlags`' bound struct fields, once bound.** `cliFlags` stops being the source of truth for these fields after binding; it remains cobra's write target (cobra needs somewhere to write flag values) but `runBackup` and everything downstream reads from viper's merged view instead. Alternative considered: keep reading `cliFlags` fields and only add environment-variable resolution as a fallback when a field is at its zero value — rejected because zero-value-as-"unset" is ambiguous for `--concurrency 0` (meaning "default to NumCPU," per its own help text) and for `--archive none` where `none` is a legitimate explicit value indistinguishable from unset.

**`Config.Dest` stays a separate, config-file-only fallback beneath viper's own config-file layer for `--dest`,** not merged into viper's config keys. Viper's config file layer already reads the same YAML file for the top-level keys this change maps (`dest`, `state`, `archive`, and the rest); `Config.Forges` (token/token_env/skip_mirrors) is out of viper's remit entirely and keeps going through `LoadConfig` unchanged, per the "Forge credentials keep their own resolution" requirement.

**Environment variable names are `BACKUP_GIT_REPOS_<FLAG_NAME>`, generated from each flag's registered name** (`viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))`), not a hand-maintained list — keeps the mapping mechanical and impossible to drift from the actual flag set.

## Risks / trade-offs

[A future flag added to `addRunFlags`/`runCmd.Flags()` without a matching `viper.BindPFlag` call silently has no environment-variable form] → cover this with a test that walks `cmd.Flags()` and asserts every flag has a bound viper key, so a forgotten binding fails CI rather than shipping quietly.

[`--concurrency 0` and `--archive-dir ""` are valid explicit values that look like "unset" to a naive layering] → resolve options by asking viper whether the key was explicitly set at each layer (`viper.IsSet` per layer, or binding order) rather than by checking the merged value against a zero value.

[Two config-reading paths (viper's `ReadInConfig` and `LoadConfig`) reading the same file could drift if one parses YAML more strictly than the other] → both already use the same file and the same top-level keys; add a test asserting a representative config file parses the same `dest` whether read through `LoadConfig` or through viper, so a future YAML edge case is caught in one place, not divergently.

## Migration plan

No data migration. Existing invocations with only flags and/or a config file behave identically, since nothing currently sets `BACKUP_GIT_REPOS_*` and the config-file precedence for `--dest` doesn't change. This ships as a normal PR: add `github.com/spf13/viper` to `go.mod`, wire the bindings, update the README, add tests for the precedence order.
