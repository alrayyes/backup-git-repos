//go:build integration

package backup_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForgejoFixtures(t *testing.T) {
	f := startForgejo(t)

	var result struct {
		Data []struct {
			FullName string `json:"full_name"`
		} `json:"data"`
	}
	f.get(t, "/api/v1/repos/search?archived=false&limit=50", &result)

	names := make([]string, 0, len(result.Data))
	for _, r := range result.Data {
		names = append(names, r.FullName)
	}

	require.ElementsMatch(t, []string{
		"team/active-repo",
		"team/empty-repo",
		f.AdminUsername + "/personal",
	}, names)
}
