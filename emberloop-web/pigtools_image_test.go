package web

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

// A pig Session's read of a PNG yields an ImageContent next to the text.
func TestPigReadReturnsImageContent(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	if err := os.WriteFile(filepath.Join(dir, "a.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	shared, err := elToolsFor(SessionMeta{ID: "s", WorkDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range shared {
		if at.Name() != "read" {
			continue
		}
		params, _ := json.Marshal(map[string]string{"path": "a.png"})
		res, err := at.Execute(context.Background(), "c1", params, nil)
		if err != nil {
			t.Fatal(err)
		}
		var img *ai.ImageContent
		for _, c := range res.Content {
			if ic, ok := c.(ai.ImageContent); ok {
				img = &ic
			}
		}
		if img == nil || img.MimeType != "image/png" || img.Data == "" {
			t.Fatalf("content = %+v", res.Content)
		}
		return
	}
	t.Fatal("no read tool in the pig loadout")
}
