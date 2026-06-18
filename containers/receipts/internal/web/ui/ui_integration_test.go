package ui_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DankersW/home-lab/containers/receipts/internal/auth"
	"github.com/DankersW/home-lab/containers/receipts/internal/db/bucket"
	"github.com/DankersW/home-lab/containers/receipts/internal/db/sqlite"
	"github.com/DankersW/home-lab/containers/receipts/internal/web"
	"github.com/DankersW/home-lab/containers/receipts/internal/web/ui"
	"github.com/stretchr/testify/require"
)

// fakeBucket is an in-memory web.ObjectStore for tests (no MinIO needed).
type fakeBucket struct {
	mu   sync.Mutex
	objs map[string][]byte
	ct   map[string]string
}

func newFakeBucket() *fakeBucket {
	return &fakeBucket{objs: map[string][]byte{}, ct: map[string]string{}}
}

func (f *fakeBucket) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objs[key] = b
	f.ct[key] = contentType
	return nil
}

func (f *fakeBucket) Get(_ context.Context, key string) (io.ReadCloser, bucket.ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objs[key]
	if !ok {
		return nil, bucket.ObjectInfo{}, bucket.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), bucket.ObjectInfo{Size: int64(len(b)), ContentType: f.ct[key]}, nil
}

func (f *fakeBucket) Remove(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objs, key)
	delete(f.ct, key)
	return nil
}

func (f *fakeBucket) HealthCheck(_ context.Context) error { return nil }

func (f *fakeBucket) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objs)
}

func newTestHandler(t *testing.T) (http.Handler, *fakeBucket) {
	t.Helper()
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	objects := newFakeBucket()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	deps := web.Deps{Store: store, Objects: objects, Logger: logger, MaxUploadBytes: 25 << 20, MaxFiles: 5}
	h := auth.Middleware(false, "tester@example.com", logger, ui.New(deps).Routes())
	return h, objects
}

func do(t *testing.T, h http.Handler, method, target, contentType string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// createReceipt posts a multipart create form and returns the created receipt id.
// It forces the desktop origin so the create redirect carries /receipts/{id}.
func createReceipt(t *testing.T, h http.Handler, fields map[string]string, filename string, fileBytes []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if k == "origin" {
			continue
		}
		require.NoError(t, w.WriteField(k, v))
	}
	require.NoError(t, w.WriteField("origin", "desktop"))
	if filename != "" {
		fw, err := w.CreateFormFile("files", filename)
		require.NoError(t, err)
		_, err = fw.Write(fileBytes)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())

	rec := do(t, h, http.MethodPost, "/receipts", w.FormDataContentType(), &buf)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	loc := rec.Header().Get("HX-Redirect")
	require.True(t, strings.HasPrefix(loc, "/receipts/"), "unexpected redirect %q", loc)
	return strings.SplitN(strings.TrimPrefix(loc, "/receipts/"), "?", 2)[0]
}

func TestHomeRendersBothShells(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := do(t, h, http.MethodGet, "/", "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, "New receipt")       // mobile add screen
	require.Contains(t, body, "Household archive") // desktop sidebar
	require.Contains(t, body, "Add it now, find it later")
}

func TestCreateRequiresTitleOnly(t *testing.T) {
	h, _ := newTestHandler(t)

	// No title -> validation flash, no redirect.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("merchant", "Bauhaus"))
	require.NoError(t, w.Close())
	rec := do(t, h, http.MethodPost, "/receipts", w.FormDataContentType(), &buf)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("HX-Redirect"))
	require.Contains(t, rec.Body.String(), "title is required")

	// Title only -> created and redirected to the list (mobile origin).
	var buf2 bytes.Buffer
	w2 := multipart.NewWriter(&buf2)
	require.NoError(t, w2.WriteField("title", "Robot lawnmower"))
	require.NoError(t, w2.WriteField("origin", "mobile"))
	require.NoError(t, w2.Close())
	rec2 := do(t, h, http.MethodPost, "/receipts", w2.FormDataContentType(), &buf2)
	require.Equal(t, http.StatusNoContent, rec2.Code, rec2.Body.String())
	require.Equal(t, "/list?flash=Receipt+saved", rec2.Header().Get("HX-Redirect"))

	listRec := do(t, h, http.MethodGet, "/list", "", nil)
	require.Contains(t, listRec.Body.String(), "Robot lawnmower")
}

func TestCreateWithTagsAndFileThenDetail(t *testing.T) {
	h, objects := newTestHandler(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	id := createReceipt(t, h, map[string]string{
		"title":    "Synology NAS",
		"merchant": "Inet",
		"amount":   "6499",
		"tags":     "homelab, nas",
		"origin":   "mobile",
	}, "receipt.png", png)
	require.NotEmpty(t, id)
	require.Equal(t, 1, objects.count())

	rec := do(t, h, http.MethodGet, "/receipts/"+id, "", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, "Synology NAS")
	require.Contains(t, body, "homelab")
	require.Contains(t, body, "nas")
	require.Contains(t, body, "6\u00a0499") // grouped amount (sv-SE nbsp thousands separator)
}

func TestSearchFiltersResults(t *testing.T) {
	h, _ := newTestHandler(t)
	createReceipt(t, h, map[string]string{"title": "LG OLED TV", "merchant": "Elgiganten", "origin": "mobile"}, "", nil)
	createReceipt(t, h, map[string]string{"title": "Robot lawnmower", "merchant": "Bauhaus", "tags": "garden", "origin": "mobile"}, "", nil)

	hit := do(t, h, http.MethodGet, "/fragments/list?shell=desktop&q=lawnmower", "", nil)
	require.Equal(t, http.StatusOK, hit.Code)
	require.Contains(t, hit.Body.String(), "Robot lawnmower")
	require.NotContains(t, hit.Body.String(), "LG OLED TV")

	// Tag-name token matches via the tags join.
	byTag := do(t, h, http.MethodGet, "/fragments/list?shell=desktop&q=garden", "", nil)
	require.Contains(t, byTag.Body.String(), "Robot lawnmower")
	require.NotContains(t, byTag.Body.String(), "LG OLED TV")

	none := do(t, h, http.MethodGet, "/fragments/list?shell=desktop&q=zzzzz", "", nil)
	require.Contains(t, none.Body.String(), "Nothing found")
}

func TestEditPageHasSingleThumbsContainer(t *testing.T) {
	// Regression: the full /edit page must not render the edit-thumbs container in
	// both shells (duplicate ids would make desktop add/delete target the hidden
	// mobile gallery). Only the mobile edit screen renders it; the desktop edit
	// modal opens via fragment from the detail view instead.
	h, _ := newTestHandler(t)
	id := createReceipt(t, h, map[string]string{"title": "Mower"}, "", nil)
	rec := do(t, h, http.MethodGet, "/receipts/"+id+"/edit", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Equal(t, 1, strings.Count(body, `id="edit-thumbs-`), "exactly one edit-thumbs container")
	require.Contains(t, body, `<div id="d-modal"></div>`, "desktop edit modal must not auto-open")
}

func TestDeleteRedirectsToList(t *testing.T) {
	h, objects := newTestHandler(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	id := createReceipt(t, h, map[string]string{"title": "Dishwasher", "origin": "mobile"}, "r.png", png)
	require.Equal(t, 1, objects.count())

	rec := do(t, h, http.MethodDelete, "/receipts/"+id, "", nil)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "/list?flash=Receipt+deleted", rec.Header().Get("HX-Redirect"))
	require.Equal(t, 0, objects.count())

	miss := do(t, h, http.MethodGet, "/receipts/"+id, "", nil)
	require.Equal(t, http.StatusNotFound, miss.Code)
}
