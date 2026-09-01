//go:build integration && gitlab

package gitlab_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// gitlabShmSize is the /dev/shm size GitLab's PostgreSQL wants. Every manual
// boot of this container during development used 256 MB; Docker's 64 MB
// default was never tested and the cost of getting this wrong is a flaky
// container rather than a loud error, so it's set explicitly.
const gitlabShmSize = 256 * 1024 * 1024

// Pinned by digest as well as tag.
const image = "gitlab/gitlab-ce:19.2.2-ce.0@sha256:f7cf5de6f453623cfda9b7cc3708b1a29e82ea9be2dfaa91b5d7e7ed9aff9e6c"

// omnibusConfig tunes GitLab CE down for a test box: no bundled monitoring
// stack, single Puma process, a modest Sidekiq concurrency, and every
// optional subsystem this package's tests never touch turned off. The
// client only lists and mirrors repositories over the API and git-http, so
// CI/CD-adjacent services (registry, Pages, KAS), the individual metrics
// exporters (separate flags from prometheus_monitoring above them), mail
// and usage/gravatar network calls all just cost reconfigure and boot time
// here. grafana and mattermost keys are deliberately absent -- both were
// removed from the Linux package in 19.0, and setting either aborts
// reconfigure outright.
//
// external_url carries no port for every test except the LFS ones in
// mirror_lfs_test.go, which pin one to make the LFS batch API's
// self-reported href match the address the fixture and the mirror under
// test actually connect to. Baking that port straight into external_url
// isn't enough on its own, though -- confirmed live, by exactly the
// failure this comment now heads off: omnibus derives nginx's own listen
// port from external_url's port when one is present, so nginx ends up
// listening *inside* the container on that port instead of 80, which is
// where testcontainers.WithExposedPorts and the docker -p mapping both
// expect it, and the container never becomes reachable at all.
// nginx['listen_port'] is the decoupling omnibus offers for exactly this:
// it fixes nginx's actual listen port at 80 regardless of what
// external_url says, while external_url's port still flows into every
// href GitLab generates server-side, LFS's batch API included. Harmless
// for the no-port default case too, since nginx already listens on 80
// there.
func omnibusConfig(externalURL string) string {
	return "external_url '" + externalURL + "'; " +
		"nginx['listen_port'] = 80; " +
		"gitlab_rails['initial_root_password'] = 'Bk7v#Qz9$mN2xLp5'; " +
		"gitlab_rails['monitoring_whitelist'] = ['0.0.0.0/0']; " +
		"puma['worker_processes'] = 0; " +
		"sidekiq['max_concurrency'] = 4; " +
		"prometheus_monitoring['enable'] = false; " +
		"alertmanager['enable'] = false; " +
		"gitlab_kas['enable'] = false; " +
		"registry['enable'] = false; " +
		"gitlab_pages['enable'] = false; " +
		"node_exporter['enable'] = false; " +
		"redis_exporter['enable'] = false; " +
		"postgres_exporter['enable'] = false; " +
		"gitlab_exporter['enable'] = false; " +
		"logrotate['enable'] = false; " +
		"mailroom['enable'] = false; " +
		"gitlab_rails['smtp_enable'] = false; " +
		"gitlab_rails['gitlab_email_enabled'] = false; " +
		"gitlab_rails['usage_ping_enabled'] = false; " +
		"gitlab_rails['gravatar_enabled'] = false;"
}

func TestContainerBoots(t *testing.T) {
	ctx := context.Background()

	ctr := runGitLab(ctx, t)

	connStr, err := ctr.PortEndpoint(ctx, "80/tcp", "http")
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connStr+"/-/health", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// runGitLab boots a tuned-down GitLab CE container and waits for the
// image's own HEALTHCHECK to report healthy -- confirmed against a live
// container to track real readiness, and simpler than reimplementing an
// HTTP-plus-exec probe that would only approximate the same thing.
func runGitLab(ctx context.Context, t *testing.T) testcontainers.Container {
	t.Helper()

	return runGitLabConfigured(ctx, t, "http://localhost", "")
}

// runGitLabConfigured is runGitLab's own implementation, and startWithLFS's
// in mirror_lfs_test.go: externalURL is GitLab's own view of its address,
// and fixedHostPort, when non-empty, pins the container's published host
// port to a value chosen before the container exists rather than one
// Docker assigns at random -- what startWithLFS needs to keep externalURL
// and the address the fixture and the mirror under test actually connect
// to in agreement.
func runGitLabConfigured(ctx context.Context, t *testing.T, externalURL, fixedHostPort string) testcontainers.Container {
	t.Helper()

	ctr, err := testcontainers.Run(ctx, image,
		testcontainers.WithExposedPorts("80/tcp"),
		testcontainers.WithEnv(map[string]string{
			"GITLAB_OMNIBUS_CONFIG": omnibusConfig(externalURL),
		}),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.ShmSize = gitlabShmSize
			if fixedHostPort != "" {
				hc.PortBindings = network.PortMap{
					network.MustParsePort("80/tcp"): {{HostPort: fixedHostPort}},
				}
			}
		}),
		// The image's own HEALTHCHECK reports healthy long before Rails
		// actually is -- confirmed against a live container, where nginx
		// answered but every request past it got reset because
		// workhorse's upstream Rails socket didn't exist yet. ForHTTP
		// alone has the same problem: nginx can 200 or reset before the
		// app behind it is real. ForExec running an actual Rails
		// expression is what closes that gap.
		//
		// Both timeouts below are load-bearing, confirmed by watching
		// each half fail on its own:
		//   - WithWaitStrategy hardcodes a 60s *outer* deadline
		//     regardless of what the strategy itself asks for, so
		//     WithWaitStrategyAndDeadline is the one that actually
		//     honours a longer wait.
		//   - Every strategy here re-wraps the context with its *own*
		//     timeout, defaulting to 60s when WithStartupTimeout was
		//     never called on it -- the outer deadline alone doesn't
		//     reach any of them.
		testcontainers.WithWaitStrategyAndDeadline(15*time.Minute,
			wait.ForHTTP("/-/health").WithPort("80/tcp").WithStartupTimeout(15*time.Minute),
			wait.ForExec([]string{"gitlab-rails", "runner", "puts 1"}).WithStartupTimeout(15*time.Minute),
		),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	return ctr
}
