package imsg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrUnavailable = errors.New("imsg is unavailable")

type Runner struct {
	binary string
}

func NewRunner(binary string) *Runner {
	return &Runner{binary: ResolveBinary(binary)}
}

func ResolveBinary(binary string) string {
	if binary := strings.TrimSpace(binary); binary != "" {
		return binary
	}

	if binary := strings.TrimSpace(os.Getenv("CHATBUBBLES_IMSG_BIN")); binary != "" {
		return binary
	}

	for _, candidate := range defaultBinaryCandidates() {
		if isExecutableFile(candidate) {
			return candidate
		}
	}

	return "imsg"
}

func (r *Runner) Binary() string {
	return r.binary
}

func defaultBinaryCandidates() []string {
	candidates := make([]string, 0, 3)

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "..", "imsg", "bin", "imsg"))
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "imsg", "bin", "imsg"),
			filepath.Join(exeDir, "..", "..", "imsg", "bin", "imsg"),
		)
	}

	return candidates
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	return info.Mode()&0o111 != 0
}

func (r *Runner) Version(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "--help")
	if err != nil {
		return "", err
	}

	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", fmt.Errorf("unexpected imsg version output: %q", line)
	}

	return fields[1], nil
}

func (r *Runner) ListChats(ctx context.Context, limit int) ([]Chat, error) {
	out, err := r.run(ctx, "chats", "--limit", strconv.Itoa(limit), "--json")
	if err != nil {
		return nil, err
	}

	var chats []Chat
	if err := decodeLines(out, &chats); err != nil {
		return nil, err
	}

	return chats, nil
}

func (r *Runner) ListMessages(ctx context.Context, chatID int64, opts ListMessagesOptions) ([]Message, error) {
	args := []string{
		"history",
		"--chat-id", strconv.FormatInt(chatID, 10),
		"--limit", strconv.Itoa(opts.Limit),
		"--json",
	}

	if opts.Attachments {
		args = append(args, "--attachments")
	}

	if opts.After != nil {
		args = append(args, "--start", opts.After.UTC().Format(timeLayout))
	}

	if opts.Before != nil {
		args = append(args, "--end", opts.Before.UTC().Format(timeLayout))
	}

	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := decodeLines(out, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *Runner) SendMessage(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	args := []string{
		"send",
		"--to", req.To,
		"--service", req.Service,
		"--json",
	}
	if strings.TrimSpace(req.Text) != "" {
		args = append(args, "--text", req.Text)
	}

	out, err := r.run(ctx, args...)
	if err != nil {
		return SendMessageResult{}, err
	}

	result := SendMessageResult{
		Status:  "sent",
		To:      req.To,
		Service: req.Service,
	}

	var parsed SendMessageResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &parsed); err == nil {
		if parsed.Status != "" {
			result.Status = parsed.Status
		}
		if parsed.To != "" {
			result.To = parsed.To
		}
		if parsed.Service != "" {
			result.Service = parsed.Service
		}
	}

	return result, nil
}

func (r *Runner) SendAttachment(ctx context.Context, req SendAttachmentRequest) (SendAttachmentResult, error) {
	args := []string{
		"send",
		"--to", req.To,
		"--file", req.FilePath,
		"--service", req.Service,
		"--json",
	}
	if strings.TrimSpace(req.Text) != "" {
		args = append(args, "--text", req.Text)
	}

	out, err := r.run(ctx, args...)
	if err != nil {
		return SendAttachmentResult{}, err
	}

	result := SendAttachmentResult{
		Status:  "sent",
		To:      req.To,
		Service: req.Service,
	}

	var parsed SendAttachmentResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &parsed); err == nil {
		if parsed.Status != "" {
			result.Status = parsed.Status
		}
		if parsed.To != "" {
			result.To = parsed.To
		}
		if parsed.Service != "" {
			result.Service = parsed.Service
		}
		if parsed.Attachment.Filename != "" || parsed.Attachment.ID != "" {
			result.Attachment = parsed.Attachment
		}
	}

	return result, nil
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.binary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}

		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrUnavailable, msg)
		}

		return nil, fmt.Errorf("imsg %s failed: %s", strings.Join(args, " "), msg)
	}

	return stdout.Bytes(), nil
}

func decodeLines[T any](raw []byte, dst *[]T) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))

	for {
		var item T
		if err := decoder.Decode(&item); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}

		*dst = append(*dst, item)
	}
}
