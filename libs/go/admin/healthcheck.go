package admin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HealthcheckArg is the subcommand that turns a service binary into its own
// probe: `/service healthcheck` exits 0 if /readyz answers 200. Distroless
// images have no shell or curl, so docker compose healthchecks call this.
const HealthcheckArg = "healthcheck"

// IsHealthcheck reports whether the process was started as a probe.
func IsHealthcheck(args []string) bool {
	return len(args) > 1 && args[1] == HealthcheckArg
}

// Probe requests /readyz on the local admin address (e.g. ":8081").
func Probe(ctx context.Context, adminAddr string) error {
	host, port, err := net.SplitHostPort(adminAddr)
	if err != nil {
		return fmt.Errorf("admin addr %q: %w", adminAddr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := "http://" + net.JoinHostPort(host, port) + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: status %d", url, resp.StatusCode)
	}
	return nil
}
