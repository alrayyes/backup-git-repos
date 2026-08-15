package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

func TestLoadConfigReadsForges(t *testing.T) {
	t.Setenv("TEST_FORGEJO_TOKEN", "secret")
	path := writeConfig(t, `
dest: /srv/backups
forges:
  - name: home
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_FORGEJO_TOKEN
`)

	cfg, err := backup.LoadConfig(path)
	require.NoError(t, err)

	require.Equal(t, []backup.ForgeConfig{{
		Name:     "home",
		Kind:     "forgejo",
		URL:      "https://git.example.org",
		TokenEnv: "TEST_FORGEJO_TOKEN",
		Token:    "secret",
	}}, cfg.Forges)
}

func TestLoadConfigErrorsOnMissingToken(t *testing.T) {
	path := writeConfig(t, `
dest: /srv/backups
forges:
  - name: home
    kind: forgejo
    url: https://git.example.org
    token_env: TEST_FORGEJO_TOKEN_UNSET
`)

	_, err := backup.LoadConfig(path)

	require.ErrorIs(t, err, backup.ErrMissingToken)
}

func TestLoadConfigErrorsOnUnknownKind(t *testing.T) {
	t.Setenv("TEST_FORGEJO_TOKEN", "secret")
	path := writeConfig(t, `
dest: /srv/backups
forges:
  - name: home
    kind: bogus
    url: https://git.example.org
    token_env: TEST_FORGEJO_TOKEN
`)

	_, err := backup.LoadConfig(path)

	var unknownKind *backup.UnknownKindError
	require.ErrorAs(t, err, &unknownKind)
	require.Equal(t, "bogus", unknownKind.Kind)
}
