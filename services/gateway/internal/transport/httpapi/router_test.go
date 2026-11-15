package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/lifecycle"

	"github.com/sokolovlev/polka/services/gateway/internal/transport/httpapi"
)

func serve(t *testing.T, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := httpapi.NewRouter(buildinfo.Get("gateway"), &lifecycle.Readiness{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, path, nil))
	return rec
}

func TestServiceRoutes(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz", "/version"} {
		rec := serve(t, http.MethodGet, path)
		assert.Equal(t, http.StatusOK, rec.Code, path)
	}
}

func TestUnknownAPIRouteIsProblemJSON(t *testing.T) {
	rec := serve(t, http.MethodGet, "/api/v1/nope")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p httpapi.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	assert.Equal(t, "NOT_FOUND", p.Code)
	assert.Equal(t, http.StatusNotFound, p.Status)
}
