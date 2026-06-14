package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dankers/home-lab/services/receipts/internal/auth"
	"github.com/dankers/home-lab/services/receipts/internal/db/bucket"
	"github.com/dankers/home-lab/services/receipts/internal/db/sqlite"
	"github.com/dankers/home-lab/services/receipts/internal/web"
	"github.com/dankers/home-lab/services/receipts/internal/web/api"
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
	h := auth.Middleware(false, "tester@example.com", logger, api.New(deps).Routes())
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

func TestAPIReceiptLifecycle(t *testing.T) {
	h, objects := newTestHandler(t)

	// Create a receipt.
	create := `{"merchant":"Coolblue","title":"OLED TV","purchase_date":"2026-02-20",` +
		`"amount":"899.00","tags":["electronics"]}`
	rec := do(t, h, http.MethodPost, "/receipts", "application/json", bytes.NewBufferString(create))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	require.Equal(t, "Coolblue", created["merchant"])
	require.Equal(t, float64(89900), created["amount_minor"])
	require.Equal(t, "tester@example.com", created["uploader_email"])

	// List returns the new receipt.
	rec = do(t, h, http.MethodGet, "/receipts", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var listed map[string][]map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed["receipts"], 1)

	// Upload an attachment (a tiny valid PNG).
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	body, ctype := multipartFile(t, "files", "shot.png", png)
	rec = do(t, h, http.MethodPost, "/receipts/"+id+"/attachments", ctype, body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, objects.count())

	var withFile map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &withFile))
	atts, _ := withFile["attachments"].([]any)
	require.Len(t, atts, 1)
	att := atts[0].(map[string]any)
	attID := att["id"].(string)
	require.Equal(t, "image/png", att["content_type"])

	// Stream the attachment back.
	rec = do(t, h, http.MethodGet, "/receipts/"+id+"/attachments/"+attID, "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, png, rec.Body.Bytes())

	// Export contains the receipt with its tag by name.
	rec = do(t, h, http.MethodGet, "/export", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var exp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &exp))
	require.Equal(t, float64(1), exp["version"])

	// Delete cascades and purges the object.
	rec = do(t, h, http.MethodDelete, "/receipts/"+id, "", nil)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 0, objects.count())

	rec = do(t, h, http.MethodGet, "/receipts/"+id, "", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAPIRejectsBadInput(t *testing.T) {
	h, _ := newTestHandler(t)

	// Missing merchant.
	rec := do(t, h, http.MethodPost, "/receipts", "application/json",
		bytes.NewBufferString(`{"purchase_date":"2026-02-20","amount":"5.00"}`))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Spoofed upload (declared png name, but bytes are plain text) is rejected.
	h2, _ := newTestHandler(t)
	rec = do(t, h2, http.MethodPost, "/receipts", "application/json",
		bytes.NewBufferString(`{"merchant":"X","purchase_date":"2026-01-01","amount":"1.00"}`))
	require.Equal(t, http.StatusCreated, rec.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	id := created["id"].(string)

	body, ctype := multipartFile(t, "files", "evil.png", []byte("this is not an image at all"))
	rec = do(t, h2, http.MethodPost, "/receipts/"+id+"/attachments", ctype, body)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func multipartFile(t *testing.T, field, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}
