// Package auth resolves the current user from the Cloudflare Access header and
// exposes it through the request context. There is no in-app login: Cloudflare
// Access authenticates at the edge and the service trusts the header because it
// is only reachable through Traefik (it publishes no ports).
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// accessEmailHeader is injected by Cloudflare Access. Go canonicalizes header
// keys, so this exact casing is what http.Header.Get matches.
const accessEmailHeader = "Cf-Access-Authenticated-User-Email"

// User identifies the authenticated household member.
type User struct {
	Email string
}

type ctxKey struct{}

// CurrentUser returns the user stored in ctx, or the zero User if none is set.
func CurrentUser(ctx context.Context) User {
	u, _ := ctx.Value(ctxKey{}).(User)
	return u
}

func withUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// Middleware resolves the current user for every request and stores it in the
// context. When requireAuth is false it falls back to devEmail (local dev);
// when requireAuth is true only the Cloudflare header is trusted. A request
// with no resolvable identity is rejected with 401.
func Middleware(requireAuth bool, devEmail string, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := strings.TrimSpace(r.Header.Get(accessEmailHeader))
		if email == "" && !requireAuth {
			email = strings.TrimSpace(devEmail)
		}
		if email == "" {
			logger.Warn("rejecting unauthenticated request", "path", r.URL.Path, "remote", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), User{Email: email})))
	})
}
