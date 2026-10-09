package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/itzikyis/docflow-ai/internal/document"
)

var (
	pdfContent = []byte("%PDF-1.7\n" + strings.Repeat("0", 1000))
	pngContent = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 100)...)
)

type stubDispatcher struct{ err error }

func (d stubDispatcher) Dispatch(context.Context, document.Document) error { return d.err }

func newDocumentsAPI(t *testing.T, maxUploadBytes int64, dispatchErr error) http.Handler {
	t.Helper()
	svc := document.NewService(document.NewMemoryRepository(), stubDispatcher{dispatchErr})
	return NewHandler(Deps{
		Logger:         discardLogger,
		Health:         NewHealth(discardLogger, nil),
		Documents:      svc,
		MaxUploadBytes: maxUploadBytes,
	})
}

type filePart struct {
	field, fileName, declaredType string
	// encodedFileName is sent as an RFC 2231 extended parameter
	// (filename*=UTF-8''...), which the multipart parser percent-decodes.
	encodedFileName string
	content         []byte
}

func multipartBody(t *testing.T, parts ...filePart) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		disposition := `form-data; name="` + p.field + `"`
		if p.fileName != "" {
			disposition += `; filename="` + p.fileName + `"`
		}
		if p.encodedFileName != "" {
			disposition += `; filename*=UTF-8''` + p.encodedFileName
		}
		h.Set("Content-Disposition", disposition)
		if p.declaredType != "" {
			h.Set("Content-Type", p.declaredType)
		}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(p.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func do(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func uploadRequest(t *testing.T, body io.Reader, contentType string) *http.Request {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/documents", body)
	req.Header.Set("Content-Type", contentType)
	return req
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return v
}

func TestUploadAndRetrieveDocument(t *testing.T) {
	api := newDocumentsAPI(t, 1<<20, nil)
	body, ct := multipartBody(t,
		filePart{field: "note", content: []byte("ignored form field")},
		filePart{field: "file", fileName: "invoice-2026.pdf", declaredType: "application/octet-stream", content: pdfContent},
	)

	rec := do(t, api, uploadRequest(t, body, ct))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body)
	}
	up := decodeJSON[uploadResponse](t, rec)
	if _, err := uuid.Parse(up.ID); err != nil {
		t.Fatalf("id %q is not a UUID", up.ID)
	}
	if loc := rec.Header().Get("Location"); loc != "/documents/"+up.ID {
		t.Errorf("Location = %q, want /documents/%s", loc, up.ID)
	}
	if up.Status != document.StatusUploaded || up.StatusURL != "/documents/"+up.ID+"/status" {
		t.Errorf("upload response = %+v", up)
	}

	rec = do(t, api, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/documents/"+up.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", rec.Code, http.StatusOK)
	}
	doc := decodeJSON[documentResponse](t, rec)
	sum := sha256.Sum256(pdfContent)
	want := documentResponse{
		ID:          up.ID,
		FileName:    "invoice-2026.pdf",
		ContentType: "application/pdf",
		SizeBytes:   int64(len(pdfContent)),
		SHA256:      hex.EncodeToString(sum[:]),
		Status:      document.StatusUploaded,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
	}
	if doc != want {
		t.Errorf("document = %+v, want %+v", doc, want)
	}
	if doc.CreatedAt.IsZero() {
		t.Error("createdAt is zero")
	}

	rec = do(t, api, httptest.NewRequestWithContext(t.Context(), http.MethodGet, up.StatusURL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status endpoint = %d, want %d", rec.Code, http.StatusOK)
	}
	if st := decodeJSON[statusResponse](t, rec); st.ID != up.ID || st.Status != document.StatusUploaded {
		t.Errorf("status response = %+v", st)
	}
}

func TestUploadAcceptsImages(t *testing.T) {
	api := newDocumentsAPI(t, 1<<20, nil)
	body, ct := multipartBody(t, filePart{field: "file", fileName: "scan.png", content: pngContent})

	rec := do(t, api, uploadRequest(t, body, ct))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body)
	}
}

func TestUploadRejections(t *testing.T) {
	const limit = 2048
	tooBig := append([]byte("%PDF-1.7\n"), make([]byte, limit)...)

	tests := []struct {
		name       string
		parts      []filePart
		rawBody    string // used instead of parts when set
		rawType    string
		wantStatus int
		wantDetail string
	}{
		{
			name:       "not multipart",
			rawBody:    `{"file":"x"}`,
			rawType:    "application/json",
			wantStatus: http.StatusUnsupportedMediaType,
			wantDetail: "multipart/form-data",
		},
		{
			name:       "missing file field",
			parts:      []filePart{{field: "other", fileName: "a.pdf", content: pdfContent}},
			wantStatus: http.StatusBadRequest,
			wantDetail: `missing "file" field`,
		},
		{
			name:       "file field is not a file",
			parts:      []filePart{{field: "file", content: pdfContent}},
			wantStatus: http.StatusBadRequest,
			wantDetail: "file upload",
		},
		{
			name:       "empty file",
			parts:      []filePart{{field: "file", fileName: "a.pdf"}},
			wantStatus: http.StatusBadRequest,
			wantDetail: "empty",
		},
		{
			name:       "text file declared as PDF",
			parts:      []filePart{{field: "file", fileName: "a.pdf", declaredType: "application/pdf", content: []byte("just text")}},
			wantStatus: http.StatusUnsupportedMediaType,
			wantDetail: "unsupported document type",
		},
		{
			name:       "executable",
			parts:      []filePart{{field: "file", fileName: "a.exe", content: append([]byte("MZ"), make([]byte, 64)...)}},
			wantStatus: http.StatusUnsupportedMediaType,
			wantDetail: "unsupported document type",
		},
		{
			name:       "too large",
			parts:      []filePart{{field: "file", fileName: "big.pdf", content: tooBig}},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantDetail: "limited to 2048 bytes",
		},
		{
			name:       "raw control character in file name",
			parts:      []filePart{{field: "file", fileName: "a\x07.pdf", content: pdfContent}},
			wantStatus: http.StatusBadRequest,
			wantDetail: "malformed",
		},
		{
			name:       "encoded control characters in file name",
			parts:      []filePart{{field: "file", encodedFileName: "a%0Alevel%3DERROR.pdf", content: pdfContent}},
			wantStatus: http.StatusBadRequest,
			wantDetail: "invalid characters",
		},
		{
			name:       "file name too long",
			parts:      []filePart{{field: "file", fileName: strings.Repeat("a", 300) + ".pdf", content: pdfContent}},
			wantStatus: http.StatusBadRequest,
			wantDetail: "longer than",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newDocumentsAPI(t, limit, nil)
			var req *http.Request
			if tt.rawBody != "" {
				req = uploadRequest(t, strings.NewReader(tt.rawBody), tt.rawType)
			} else {
				body, ct := multipartBody(t, tt.parts...)
				req = uploadRequest(t, body, ct)
			}

			rec := do(t, api, req)
			assertProblem(t, rec, tt.wantStatus, tt.wantDetail)
		})
	}
}

// A body far larger than the limit is rejected from its declared length,
// before any of it is read.
func TestUploadRejectsDeclaredOversizeBody(t *testing.T) {
	api := newDocumentsAPI(t, 1024, nil)
	req := uploadRequest(t, strings.NewReader("x"), "multipart/form-data; boundary=b")
	req.ContentLength = 10 << 20

	assertProblem(t, do(t, api, req), http.StatusRequestEntityTooLarge, "limited to")
}

// Without a Content-Length (chunked upload) the size is enforced while
// streaming.
func TestUploadRejectsOversizeStream(t *testing.T) {
	api := newDocumentsAPI(t, 1024, nil)
	big := append([]byte("%PDF-1.7\n"), make([]byte, 2<<20)...)
	body, ct := multipartBody(t, filePart{field: "file", fileName: "big.pdf", content: big})
	req := uploadRequest(t, body, ct)
	req.ContentLength = -1

	assertProblem(t, do(t, api, req), http.StatusRequestEntityTooLarge, "limited to")
}

func TestUploadInternalErrorDoesNotLeakDetails(t *testing.T) {
	api := newDocumentsAPI(t, 1<<20, errors.New("pubsub: connection refused to 10.0.0.9"))
	body, ct := multipartBody(t, filePart{field: "file", fileName: "a.pdf", content: pdfContent})

	rec := do(t, api, uploadRequest(t, body, ct))
	assertProblem(t, rec, http.StatusInternalServerError, "")
	if strings.Contains(rec.Body.String(), "10.0.0.9") {
		t.Errorf("response leaks internal error: %s", rec.Body)
	}
}

func TestGetDocumentErrors(t *testing.T) {
	api := newDocumentsAPI(t, 1<<20, nil)
	unknown := uuid.NewString()

	tests := []struct {
		name, path string
		wantStatus int
		wantDetail string
	}{
		{"invalid ID", "/documents/not-a-uuid", http.StatusBadRequest, "must be a UUID"},
		{"invalid ID on status", "/documents/123/status", http.StatusBadRequest, "must be a UUID"},
		{"unknown ID", "/documents/" + unknown, http.StatusNotFound, unknown},
		{"unknown ID on status", "/documents/" + unknown + "/status", http.StatusNotFound, unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, api, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))
			assertProblem(t, rec, tt.wantStatus, tt.wantDetail)
		})
	}
}

func assertProblem(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantDetail string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	p := decodeJSON[problem](t, rec)
	if p.Status != wantStatus || p.Title != http.StatusText(wantStatus) || p.Type != "about:blank" {
		t.Errorf("problem = %+v", p)
	}
	if !strings.Contains(p.Detail, wantDetail) {
		t.Errorf("detail = %q, want it to contain %q", p.Detail, wantDetail)
	}
	if p.RequestID == "" || p.RequestID != rec.Header().Get(requestIDHeader) {
		t.Errorf("requestId = %q, want the X-Request-ID header %q", p.RequestID, rec.Header().Get(requestIDHeader))
	}
}
