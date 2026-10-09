package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/itzikyis/docflow-ai/internal/document"
)

const (
	uploadField = "file"

	// sniffLength is the most bytes http.DetectContentType looks at.
	sniffLength = 512

	// multipartOverhead allows for the boundaries, part headers and other
	// form fields around the file within the request body limit.
	multipartOverhead = 1 << 20

	maxFileNameLength = 255
)

// acceptedContentTypes are the document types accepted for processing. The
// type is detected from the file's content; the client's declared type is
// ignored because it is trivially spoofed.
var acceptedContentTypes = map[string]bool{
	"application/pdf": true,
	"image/png":       true,
	"image/jpeg":      true,
}

type documentHandlers struct {
	svc            *document.Service
	logger         *slog.Logger
	maxUploadBytes int64
}

type uploadResponse struct {
	ID        string          `json:"id"`
	Status    document.Status `json:"status"`
	StatusURL string          `json:"statusUrl"`
}

type documentResponse struct {
	ID          string          `json:"id"`
	FileName    string          `json:"fileName"`
	ContentType string          `json:"contentType"`
	SizeBytes   int64           `json:"sizeBytes"`
	SHA256      string          `json:"sha256"`
	Status      document.Status `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type statusResponse struct {
	ID        string          `json:"id"`
	Status    document.Status `json:"status"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// upload handles POST /documents. The file is streamed: it is never held in
// memory or written to a temporary file, whatever its size.
func (h *documentHandlers) upload(w http.ResponseWriter, r *http.Request) {
	limit := h.maxUploadBytes + multipartOverhead
	if r.ContentLength > limit {
		h.tooLarge(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	upload, err := h.readUpload(r)
	if err != nil {
		h.uploadFailed(w, r, err)
		return
	}

	doc, err := h.svc.Register(r.Context(), upload)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "register document failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "")
		return
	}
	h.logger.InfoContext(r.Context(), "document uploaded",
		"document_id", doc.ID.String(),
		"content_type", doc.ContentType,
		"size_bytes", doc.SizeBytes)

	location := "/documents/" + doc.ID.String()
	w.Header().Set("Location", location)
	writeJSON(w, http.StatusAccepted, uploadResponse{
		ID:        doc.ID.String(),
		Status:    doc.Status,
		StatusURL: location + "/status",
	})
}

// clientError is an upload problem caused by the request itself.
type clientError struct {
	status int
	detail string
}

func (e *clientError) Error() string { return e.detail }

func (h *documentHandlers) readUpload(r *http.Request) (document.Upload, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return document.Upload{}, &clientError{http.StatusUnsupportedMediaType,
			fmt.Sprintf("request must be multipart/form-data with a %q field", uploadField)}
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return document.Upload{}, &clientError{http.StatusBadRequest,
				fmt.Sprintf("missing %q field", uploadField)}
		}
		if err != nil {
			return document.Upload{}, err
		}
		if part.FormName() == uploadField {
			return h.readFile(part)
		}
	}
}

func (h *documentHandlers) readFile(part *multipart.Part) (document.Upload, error) {
	name := part.FileName()
	if err := validateFileName(name); err != nil {
		return document.Upload{}, err
	}

	head := make([]byte, sniffLength)
	n, err := io.ReadFull(part, head)
	switch {
	case errors.Is(err, io.EOF):
		return document.Upload{}, &clientError{http.StatusBadRequest, "file is empty"}
	case err != nil && !errors.Is(err, io.ErrUnexpectedEOF):
		return document.Upload{}, err
	}
	head = head[:n]

	contentType, _, _ := strings.Cut(http.DetectContentType(head), ";")
	if !acceptedContentTypes[contentType] {
		return document.Upload{}, &clientError{http.StatusUnsupportedMediaType,
			"unsupported document type; accepted types are PDF, PNG and JPEG"}
	}

	hash := sha256.New()
	hash.Write(head)
	rest, err := io.Copy(hash, io.LimitReader(part, h.maxUploadBytes-int64(n)+1))
	if err != nil {
		return document.Upload{}, err
	}
	size := int64(n) + rest
	if size > h.maxUploadBytes {
		return document.Upload{}, &http.MaxBytesError{Limit: h.maxUploadBytes}
	}

	return document.Upload{
		FileName:    name,
		ContentType: contentType,
		SizeBytes:   size,
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

// validateFileName checks the client-supplied name, which is stored and
// later shown and logged. multipart.Part.FileName already strips directories.
func validateFileName(name string) error {
	switch {
	case name == "":
		return &clientError{http.StatusBadRequest, fmt.Sprintf("%q must be a file upload with a file name", uploadField)}
	case len(name) > maxFileNameLength:
		return &clientError{http.StatusBadRequest, fmt.Sprintf("file name is longer than %d bytes", maxFileNameLength)}
	case !utf8.ValidString(name) || strings.ContainsFunc(name, unicode.IsControl):
		return &clientError{http.StatusBadRequest, "file name contains invalid characters"}
	}
	return nil
}

func (h *documentHandlers) uploadFailed(w http.ResponseWriter, r *http.Request, err error) {
	var ce *clientError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &ce):
		writeProblem(w, r, ce.status, ce.detail)
	case errors.As(err, &tooLarge):
		h.tooLarge(w, r)
	default:
		h.logger.WarnContext(r.Context(), "reading upload failed", "error", err)
		writeProblem(w, r, http.StatusBadRequest, "malformed multipart request body")
	}
}

func (h *documentHandlers) tooLarge(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusRequestEntityTooLarge,
		fmt.Sprintf("documents are limited to %d bytes", h.maxUploadBytes))
}

// get handles GET /documents/{id}.
func (h *documentHandlers) get(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, documentResponse{
		ID:          doc.ID.String(),
		FileName:    doc.FileName,
		ContentType: doc.ContentType,
		SizeBytes:   doc.SizeBytes,
		SHA256:      doc.SHA256,
		Status:      doc.Status,
		CreatedAt:   doc.CreatedAt,
		UpdatedAt:   doc.UpdatedAt,
	})
}

// status handles GET /documents/{id}/status, a small response for clients
// polling until processing finishes.
func (h *documentHandlers) status(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.lookup(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{
		ID:        doc.ID.String(),
		Status:    doc.Status,
		UpdatedAt: doc.UpdatedAt,
	})
}

// lookup loads the document named by the {id} path value, writing an error
// response and returning false if it cannot.
func (h *documentHandlers) lookup(w http.ResponseWriter, r *http.Request) (document.Document, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "document ID must be a UUID")
		return document.Document{}, false
	}

	doc, err := h.svc.Get(r.Context(), id)
	switch {
	case errors.Is(err, document.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, fmt.Sprintf("no document with ID %s", id))
		return document.Document{}, false
	case err != nil:
		h.logger.ErrorContext(r.Context(), "get document failed", "document_id", id.String(), "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "")
		return document.Document{}, false
	}
	return doc, true
}
