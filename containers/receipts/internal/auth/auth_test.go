package auth_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DankersW/home-lab/containers/receipts/internal/auth"
	"github.com/stretchr/testify/require"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		requireAuth bool
		devEmail    string
		header      string
		wantStatus  int
		wantEmail   string
	}{
		{name: "cloudflare header trusted", requireAuth: true, header: "wouter@example.com", wantStatus: http.StatusOK, wantEmail: "wouter@example.com"},
		{name: "no header rejected in prod", requireAuth: true, wantStatus: http.StatusUnauthorized},
		{name: "dev fallback used", requireAuth: false, devEmail: "dev@example.com", wantStatus: http.StatusOK, wantEmail: "dev@example.com"},
		{name: "header wins over dev fallback", requireAuth: false, devEmail: "dev@example.com", header: "real@example.com", wantStatus: http.StatusOK, wantEmail: "real@example.com"},
		{name: "dev fallback ignored when auth required", requireAuth: true, devEmail: "dev@example.com", wantStatus: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotEmail string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotEmail = auth.CurrentUser(r.Context()).Email
				w.WriteHeader(http.StatusOK)
			})
			h := auth.Middleware(tc.requireAuth, tc.devEmail, discardLogger(), next)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Cf-Access-Authenticated-User-Email", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantEmail, gotEmail)
		})
	}
}
