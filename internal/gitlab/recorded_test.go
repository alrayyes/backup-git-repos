//go:build integration

package gitlab_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
	"github.com/stretchr/testify/require"
)

const testdataDir = "testdata"

// TestRecordedListerContract runs the same contract suite the real GitLab
// container satisfies (lister_test.go, gitlab tag), but against JSON
// recorded from a real container rather than booting one. GitLab CE takes
// several minutes to boot, too long for every pull request, so this is what
// gives the adapter coverage on every one of them; refresh the recording
// with `go test -tags='integration gitlab' -run TestUpdateFixtures -update`.
func TestRecordedListerContract(t *testing.T) {
	active := readFixture(t, "projects_active.json")
	archived := readFixture(t, "projects_archived.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("archived") == "true" {
			_, _ = w.Write(archived)
			return
		}
		_, _ = w.Write(active)
	}))
	defer srv.Close()

	backup.TestLister(t, func(*testing.T) backup.Lister {
		client, err := gitlab.New(srv.URL, "unused")
		require.NoError(t, err)
		return client
	})
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, name))
	require.NoError(t, err)
	return data
}
