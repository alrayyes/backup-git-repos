//go:build integration

package forgejo_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcforgejo "github.com/testcontainers/testcontainers-go/modules/forgejo"
)

// Pinned by digest as well as tag: the module's own examples pin :11, so this
// is the first thing to fail if 16.0.3 behaves differently.
const image = "codeberg.org/forgejo/forgejo:16.0.3@sha256:7c4e1db440be7b2ca685b49d0d7864cdd78e92431f531bf7893659def8200fc5"

func TestContainerBoots(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcforgejo.Run(ctx, image)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	connStr, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connStr+"/api/healthz", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
