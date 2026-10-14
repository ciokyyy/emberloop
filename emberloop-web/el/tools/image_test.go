package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 1x1 transparent PNG.
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func TestReadReturnsImageAttachment(t *testing.T) {
	dir := t.TempDir()
	raw, _ := base64.StdEncoding.DecodeString(tinyPNG)
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "pic.png"})
	text, imgs, err := ExecuteMulti(context.Background(), readTool{}, dir, args)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 || imgs[0].MimeType != "image/png" || imgs[0].Data != tinyPNG {
		t.Fatalf("images = %+v", imgs)
	}
	if !strings.Contains(text, "pic.png") || !strings.Contains(text, "attached") {
		t.Fatalf("text = %q", text)
	}
	// The text-only path says it cannot show the picture, never dumps bytes.
	plain, err := readTool{}.Execute(context.Background(), dir, args)
	if err != nil || !strings.Contains(plain, "cannot show images") {
		t.Fatalf("plain = %q, %v", plain, err)
	}
}

func TestReadImageExtensionWithTextContentReadsAsText(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fake.png"), []byte("just text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "fake.png"})
	_, imgs, err := ExecuteMulti(context.Background(), readTool{}, dir, args)
	if err != nil || len(imgs) != 0 {
		t.Fatalf("imgs = %v, err = %v", imgs, err)
	}
}

func TestReadImageOverLimitRefused(t *testing.T) {
	dir := t.TempDir()
	big := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxImageBytes)...)
	_ = os.WriteFile(filepath.Join(dir, "big.png"), big, 0o644)
	args, _ := json.Marshal(map[string]string{"path": "big.png"})
	if _, _, err := ExecuteMulti(context.Background(), readTool{}, dir, args); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("err = %v", err)
	}
}
