// Package httpapi is the public REST/JSON transport of the gateway (spec 4.2,
// 6.5). Stage 0 serves only the service routes; /api/v1 handlers are added
// stage by stage, following the OpenAPI spec in services/gateway/api.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sokolovlev/polka/libs/go/admin"
	"github.com/sokolovlev/polka/libs/go/buildinfo"
	"github.com/sokolovlev/polka/libs/go/lifecycle"
)

// Problem is an RFC 9457 Problem Details body with the machine-readable code
// from spec 6.5.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Code   string `json:"code"`
}

// NewRouter builds the gateway's HTTP handler.
func NewRouter(info buildinfo.Info, ready *lifecycle.Readiness, checks ...admin.Check) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)

	svc := admin.Handler(info, ready, checks...)
	r.Method(http.MethodGet, "/healthz", svc)
	r.Method(http.MethodGet, "/readyz", svc)
	r.Method(http.MethodGet, "/version", svc)

	r.Route("/api/v1", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeProblem(w, Problem{Type: "about:blank", Title: "Not Found", Status: http.StatusNotFound, Code: "NOT_FOUND"})
		})
	})
	return r
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
