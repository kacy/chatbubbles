package attachment

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kacy/imsg-bridge/internal/imsg"
)

func TestServiceEncodeAndOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(path, []byte("hello from attachment"), 0o600); err != nil {
		t.Fatalf("write attachment file: %v", err)
	}

	service := NewService([]byte("secret"), filepath.Join(dir, "tmp"))
	id, err := service.Encode(imsg.Attachment{
		Path:      path,
		Filename:  "photo.jpg",
		MIMEType:  "image/jpeg",
		SizeBytes: 21,
	})
	if err != nil {
		t.Fatalf("encode attachment: %v", err)
	}

	file, meta, err := service.Open(id)
	if err != nil {
		t.Fatalf("open attachment: %v", err)
	}
	defer file.Close()

	if meta.Filename != "photo.jpg" {
		t.Fatalf("expected filename photo.jpg, got %q", meta.Filename)
	}
	if meta.MIMEType != "image/jpeg" {
		t.Fatalf("expected mime image/jpeg, got %q", meta.MIMEType)
	}
}

func TestSaveUploadStoresFile(t *testing.T) {
	dir := t.TempDir()
	service := NewService([]byte("secret"), filepath.Join(dir, "tmp"))

	file, err := service.SaveUpload("hello.txt", "", strings.NewReader("hello world"))
	if err != nil {
		t.Fatalf("save upload: %v", err)
	}

	body, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatalf("read saved upload: %v", err)
	}

	if string(body) != "hello world" {
		t.Fatalf("unexpected saved body %q", string(body))
	}
	if file.MIMEType == "" {
		t.Fatal("expected mime type to be inferred")
	}
}

func TestOpenRejectsInvalidID(t *testing.T) {
	service := NewService([]byte("secret"), t.TempDir())
	_, _, err := service.Open("bad")
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected invalid id, got %v", err)
	}
}
