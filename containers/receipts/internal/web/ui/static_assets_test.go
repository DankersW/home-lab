package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DankersW/home-lab/containers/receipts/internal/web/ui"
	"github.com/stretchr/testify/require"
)

// TestStaticServesScannerAssets guards that the scanner controller and its
// vendored engine are embedded and served, so a build can't silently ship
// without the ~9 MB OpenCV/jscanify bundle the scan feature depends on.
func TestStaticServesScannerAssets(t *testing.T) {
	h := ui.StaticHandler()
	cases := []struct{ path, wantSubstr string }{
		{"/static/scanner.js", "data-scan-input"},
		{"/static/vendor/jscanify.js", "jscanify"},
		{"/static/vendor/opencv.js", "WebAssembly"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusOK, rec.Code, "GET %s", c.path)
		require.NotEmpty(t, rec.Body.Bytes(), "GET %s body", c.path)
		require.Containsf(t, rec.Body.String(), c.wantSubstr, "GET %s content", c.path)
	}
}

// TestAddScreenRendersScanControls guards the scan entry points and script tag
// on the page a fresh visit lands on.
func TestAddScreenRendersScanControls(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := do(t, h, http.MethodGet, "/", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "Scan document")
	require.Contains(t, body, "data-scan")
	require.Contains(t, body, "data-scan-input")
	require.Contains(t, body, "/static/scanner.js")
}
