package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kacy/imsg-bridge/internal/attachment"
	"github.com/kacy/imsg-bridge/internal/imsg"
)

func (s *Server) handleSendAttachment(w http.ResponseWriter, r *http.Request) {
	if s.attachments == nil {
		writeError(w, http.StatusInternalServerError, "internal", "attachment service is not configured")
		return
	}

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request must be multipart form data")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	to := strings.TrimSpace(r.FormValue("to"))
	text := strings.TrimSpace(r.FormValue("text"))
	service := strings.ToLower(strings.TrimSpace(r.FormValue("service")))
	if service == "" {
		service = "auto"
	}

	if to == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "to is required")
		return
	}

	switch service {
	case "auto", "imessage", "sms":
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "service must be one of auto, imessage, or sms")
		return
	}

	fileBody, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "file is required")
		return
	}
	defer fileBody.Close()

	stored, err := s.attachments.SaveUpload(fileHeader.Filename, fileHeader.Header.Get("Content-Type"), fileBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	defer os.Remove(stored.Path)

	result, err := s.runner.SendAttachment(r.Context(), imsg.SendAttachmentRequest{
		To:       to,
		Text:     text,
		FilePath: stored.Path,
		Service:  service,
	})
	if err != nil {
		writeError(w, statusForErr(err), "internal", err.Error())
		return
	}

	if result.Attachment.Filename == "" {
		result.Attachment.Filename = stored.Filename
	}
	if result.Attachment.MIMEType == "" {
		result.Attachment.MIMEType = stored.MIMEType
	}
	if result.Attachment.SizeBytes == 0 {
		result.Attachment.SizeBytes = stored.SizeBytes
	}

	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleGetAttachment(w http.ResponseWriter, r *http.Request) {
	if s.attachments == nil {
		writeError(w, http.StatusInternalServerError, "internal", "attachment service is not configured")
		return
	}

	file, meta, err := s.attachments.Open(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		switch {
		case errors.Is(err, attachment.ErrInvalidID), errors.Is(err, attachment.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "attachment was not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
		}
		return
	}
	defer file.Close()

	contentType := strings.TrimSpace(meta.MIMEType)
	if contentType == "" {
		if guess := mime.TypeByExtension(filepath.Ext(meta.Filename)); guess != "" {
			contentType = guess
		}
	}
	if contentType == "" {
		head := make([]byte, 512)
		n, _ := file.Read(head)
		contentType = http.DetectContentType(head[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", contentDisposition(meta.Filename))
	if meta.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(meta.SizeBytes, 10))
	}

	if _, err := io.Copy(w, file); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
}

func contentDisposition(name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	return `attachment; filename="` + name + `"`
}
