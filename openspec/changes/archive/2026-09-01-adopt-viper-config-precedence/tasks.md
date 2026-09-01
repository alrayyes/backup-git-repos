## Dependency

- [ ] 1.1 Add `github.com/spf13/viper` to `go.mod` pinned to an exact version, run `go mod tidy`, and verify `go build ./...` succeeds

## Viper wiring

- [ ] 2.1 In `runBackup` (`cli.go`), construct a `*viper.Viper`, call `SetEnvPrefix("BACKUP_GIT_REPOS")`, `AutomaticEnv()`, and `SetEnvKeyReplacer(strings.NewReplacer("-", "_"))`, and verify with a unit test that an environment variable alone (no flag, no config file) resolves a value
- [ ] 2.2 Bind every `run`/`list` flag (`dest`, `state`, `archive`, `archive-dir`, `concurrency`, `timeout`, `forge`, `verbose`, `dry-run`, `prune-removed`, `export-metadata`) to the viper instance with `BindPFlag`, and verify with a test that walks `cmd.Flags()` and asserts each is bound (design.md's flagged risk)
- [ ] 2.3 Point viper's config-file layer at the same `configPath` `resolveConfigPath` already resolves (`SetConfigFile` + `ReadInConfig`), tolerating a missing file the same way `LoadConfig` already requires one to exist by the time it's called, and verify with a test that a value present only in the config file resolves through viper

## Precedence resolution

- [ ] 3.1 Replace direct `cliFlags` field reads in `runBackup`/`resolveDestPaths`/`runForge` with reads through the viper instance (`GetString`, `GetInt`, `GetDuration`, `GetStringSlice`, `GetBool`), and verify `go test ./...` still passes with existing behavior unchanged when no environment variables are set
- [ ] 3.2 Verify with a table-driven test that flag > environment variable > config file > default holds for at least `--dest`/`BACKUP_GIT_REPOS_DEST`/`dest` (string) and `--concurrency`/`BACKUP_GIT_REPOS_CONCURRENCY`/no config-file equivalent (int), including the case where a flag is explicitly set to its own zero value (`--concurrency 0`) and still wins over the environment variable and config file
- [ ] 3.3 Verify with a test that `--archive-dir ""` (an explicit empty value) is distinguished from "unset," per design.md's flagged risk

## Forge credentials unaffected

- [ ] 4.1 Verify with a test that `token_env` resolution in `LoadConfig` is unchanged: a forge config with `token_env` set still resolves its token from the named environment variable, independent of the new viper layering

## Documentation

- [ ] 5.1 Update README's Configuration section to document the `BACKUP_GIT_REPOS_<FLAG_NAME>` environment-variable form for every option, alongside its existing flag and config-file forms, and the flag > environment variable > config file > default precedence
