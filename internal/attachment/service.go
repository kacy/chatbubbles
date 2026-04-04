package attachment

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kacy/imsg-bridge/internal/imsg"
)

var ErrInvalidID = errors.New("attachment id is invalid")
var ErrNotFound = errors.New("attachment file was not found")

type Service struct {
	key    [32]byte
	tmpDir string
}

type File struct {
	ID        string
	Path      string
	Filename  string
	MIMEType  string
	SizeBytes int64
}

type descriptor struct {
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	MIMEType  string `json:"mime_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

func NewService(secret []byte, tmpDir string) *Service {
	sum := sha256.Sum256(secret)
	return &Service{
		key:    sum,
		tmpDir: tmpDir,
	}
}

func (s *Service) DecorateMessages(messages []imsg.Message) {
	for i := range messages {
		for j := range messages[i].Attachments {
			if messages[i].Attachments[j].ID != "" {
				continue
			}

			id, err := s.Encode(messages[i].Attachments[j])
			if err != nil {
				continue
			}

			messages[i].Attachments[j].ID = id
		}
	}
}

func (s *Service) Encode(att imsg.Attachment) (string, error) {
	if s == nil || strings.TrimSpace(att.Path) == "" {
		return "", ErrInvalidID
	}

	return s.encodeDescriptor(descriptor{
		Path:      strings.TrimSpace(att.Path),
		Filename:  strings.TrimSpace(att.Filename),
		MIMEType:  strings.TrimSpace(att.MIMEType),
		SizeBytes: att.SizeBytes,
	})
}

func (s *Service) SaveUpload(filename string, mimeType string, body io.Reader) (File, error) {
	if s == nil {
		return File{}, errors.New("attachment service is not configured")
	}

	filename = sanitizeFilename(filename)
	if filename == "" {
		return File{}, errors.New("attachment filename is required")
	}

	if err := os.MkdirAll(s.tmpDir, 0o700); err != nil {
		return File{}, fmt.Errorf("create attachment temp dir: %w", err)
	}

	path := filepath.Join(s.tmpDir, randomName()+filepath.Ext(filename))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return File{}, fmt.Errorf("create attachment temp file: %w", err)
	}

	reader := bufio.NewReader(body)
	sniff, _ := reader.Peek(512)
	size, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return File{}, fmt.Errorf("store attachment upload: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return File{}, fmt.Errorf("close attachment upload: %w", closeErr)
	}

	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		if guess := mime.TypeByExtension(filepath.Ext(filename)); guess != "" {
			mimeType = guess
		} else if len(sniff) > 0 {
			mimeType = http.DetectContentType(sniff)
		}
	}

	return File{
		Path:      path,
		Filename:  filename,
		MIMEType:  mimeType,
		SizeBytes: size,
	}, nil
}

func (s *Service) Open(id string) (*os.File, File, error) {
	desc, err := s.decodeDescriptor(id)
	if err != nil {
		return nil, File{}, err
	}

	file, err := os.Open(desc.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, File{}, ErrNotFound
	}
	if err != nil {
		return nil, File{}, fmt.Errorf("open attachment: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, File{}, fmt.Errorf("stat attachment: %w", err)
	}

	if desc.SizeBytes == 0 {
		desc.SizeBytes = info.Size()
	}

	return file, File{
		ID:        id,
		Path:      desc.Path,
		Filename:  desc.Filename,
		MIMEType:  desc.MIMEType,
		SizeBytes: desc.SizeBytes,
	}, nil
}

func (s *Service) encodeDescriptor(desc descriptor) (string, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(desc)
	if err != nil {
		return "", fmt.Errorf("encode attachment descriptor: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate attachment nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, body, nil)
	token := append(nonce, sealed...)
	return "a_" + base64.RawURLEncoding.EncodeToString(token), nil
}

func (s *Service) decodeDescriptor(id string) (descriptor, error) {
	if s == nil {
		return descriptor{}, ErrInvalidID
	}

	if !strings.HasPrefix(id, "a_") {
		return descriptor{}, ErrInvalidID
	}

	token, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, "a_"))
	if err != nil {
		return descriptor{}, ErrInvalidID
	}

	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return descriptor{}, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return descriptor{}, err
	}

	if len(token) <= gcm.NonceSize() {
		return descriptor{}, ErrInvalidID
	}

	nonce := token[:gcm.NonceSize()]
	body := token[gcm.NonceSize():]
	raw, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return descriptor{}, ErrInvalidID
	}

	var desc descriptor
	if err := json.Unmarshal(raw, &desc); err != nil {
		return descriptor{}, ErrInvalidID
	}

	desc.Path = strings.TrimSpace(desc.Path)
	desc.Filename = sanitizeFilename(desc.Filename)
	if desc.Path == "" || desc.Filename == "" {
		return descriptor{}, ErrInvalidID
	}

	return desc, nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

func randomName() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}

	return base64.RawURLEncoding.EncodeToString(raw)
}
