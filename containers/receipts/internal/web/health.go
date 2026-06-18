package web

import "net/http"

// Health returns a handler that verifies the metadata store and object store
// are reachable. It requires no authentication.
func Health(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := deps.Store.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := deps.Objects.HealthCheck(ctx); err != nil {
			http.Error(w, "object store unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	}
}
