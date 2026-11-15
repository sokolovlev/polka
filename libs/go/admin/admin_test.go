package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
)

func get(t *testing.T, h http.Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	return rec.Code, body
}

func TestHealthzAlwaysOK(t *testing.T) {
	failing := admin.Check{Name: "db", Check: func(context.Context) error { return errors.New("down") }}
	h := admin.Handler(buildinfo.Get("svc"), &lifecycle.Readiness{}, failing)

	code, _ := get(t, h, "/healthz")
	assert.Equal(t, http.StatusOK, code)
}

func TestReadyzReportsFailedChecks(t *testing.T) {
	ok := admin.Check{Name: "kafka", Check: func(context.Context) error { return nil }}
	failing := admin.Check{Name: "db", Check: func(context.Context) error { return errors.New("down") }}

	code, _ := get(t, admin.Handler(buildinfo.Get("svc"), nil, ok), "/readyz")
	assert.Equal(t, http.StatusOK, code)

	code, body := get(t, admin.Handler(buildinfo.Get("svc"), nil, ok, failing), "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, map[string]any{"db": "down"}, body["failed"])
}

func TestVersion(t *testing.T) {
	code, body := get(t, admin.Handler(buildinfo.Get("gateway"), nil), "/version")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "gateway", body["service"])
	assert.Equal(t, "dev", body["version"])
	assert.Contains(t, body, "commit")
}

func TestHTTPServerServesAndShutsDown(t *testing.T) {
	srv, err := admin.NewHTTPServer(context.Background(), "admin", "127.0.0.1:0", admin.Handler(buildinfo.Get("svc"), nil))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- srv.Run(context.Background()) }()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+srv.Addr()+"/healthz", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, srv.Shutdown(context.Background()))
	require.NoError(t, <-done)
}

func TestNewHTTPServerFailsOnBusyPort(t *testing.T) {
	first, err := admin.NewHTTPServer(context.Background(), "a", "127.0.0.1:0", http.NotFoundHandler())
	require.NoError(t, err)
	_, err = admin.NewHTTPServer(context.Background(), "b", first.Addr(), http.NotFoundHandler())
	require.Error(t, err)
	require.NoError(t, first.Shutdown(context.Background()))
}
