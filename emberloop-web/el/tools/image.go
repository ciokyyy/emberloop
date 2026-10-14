package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Image is one picture a tool hands back next to its text. Data is base64.
// MCP hosts get it as an image content block (claude), pig hosts as an
// ai.ImageContent in the tool result; a model without image input sees pig's
// placeholder instead.
type Image struct {
	MimeType string
	Data     string
}

// Multimodal is a tool that can return images. Execute stays the text-only
// path for hosts that cannot carry them.
type Multimodal interface {
	ExecuteMulti(ctx context.Context, root string, args json.RawMessage) (string, []Image, error)
}

// ExecuteMulti runs a tool and returns its images too, or none for a
// text-only tool.
func ExecuteMulti(ctx context.Context, t Tool, root string, args json.RawMessage) (string, []Image, error) {
	if m, ok := t.(Multimodal); ok {
		return m.ExecuteMulti(ctx, root, args)
	}
	text, err := t.Execute(ctx, root, args)
	return text, nil, err
}

// maxImageBytes is the largest image read attaches; the model APIs reject
// larger ones (5 MB for Anthropic).
const maxImageBytes = 5 << 20

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// loadImage reads target as an image when it is a local PNG, JPEG, GIF or
// WebP file. ok is false for anything else, so the caller falls through to
// the normal read. The content is sniffed, not only the extension.
func loadImage(baseDir, target string) (img Image, desc string, ok bool, err error) {
	_, isImg := imageTypes[strings.ToLower(filepath.Ext(target))]
	if !isImg || isURLTarget(target) || strings.HasPrefix(target, elScheme) {
		return Image{}, "", false, nil
	}
	if _, _, isCtx := ParseCtxHandle(target); isCtx {
		return Image{}, "", false, nil
	}
	full, err := readTarget(baseDir, target)
	if err != nil {
		return Image{}, "", false, err
	}
	info, statErr := os.Stat(full)
	if statErr != nil || !info.Mode().IsRegular() {
		return Image{}, "", false, nil
	}
	if err := refuseCredential(target, full); err != nil {
		return Image{}, "", false, err
	}
	if info.Size() > maxImageBytes {
		return Image{}, "", true, fmt.Errorf("%s: image is %d bytes, over the %d byte limit; downscale it with bash first", target, info.Size(), maxImageBytes)
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return Image{}, "", true, fmt.Errorf("%s: %w", target, err)
	}
	// Trust the content over the extension; a renamed non-image reads as text.
	got := http.DetectContentType(raw)
	if got != "image/png" && got != "image/jpeg" && got != "image/gif" && got != "image/webp" {
		return Image{}, "", false, nil
	}
	desc = fmt.Sprintf("image %s (%s, %d bytes) attached", target, got, len(raw))
	return Image{MimeType: got, Data: base64.StdEncoding.EncodeToString(raw)}, desc, true, nil
}

// ExecuteMulti reads an image file as an attached image, and anything else
// as Execute does.
func (t readTool) ExecuteMulti(ctx context.Context, root string, args json.RawMessage) (string, []Image, error) {
	var p struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(args, &p) == nil && p.Path != "" {
		img, desc, ok, err := loadImage(root, p.Path)
		if err != nil {
			return "", nil, err
		}
		if ok {
			return desc, []Image{img}, nil
		}
	}
	text, err := t.Execute(ctx, root, args)
	return text, nil, err
}
