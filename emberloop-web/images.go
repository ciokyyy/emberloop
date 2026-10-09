package web

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Image is one prompt attachment: a MIME type and base64 data. The bytes go
// to the Backend only; the Event log keeps a count.
type Image struct {
	Mime string `json:"mime"`
	Data string `json:"data"`
}

const (
	// maxImages caps attachments per message.
	maxImages = 8
	// maxImageBytes caps one decoded image.
	maxImageBytes = 5 << 20
	// maxPromptBytes caps a whole prompt request body (base64 inflates 4/3).
	maxPromptBytes = maxImages*(maxImageBytes*4/3+1024) + 1<<20
)

var imageMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// validateImages returns the why-not for a bad attachment list, "" when fine.
func validateImages(images []Image) string {
	if len(images) > maxImages {
		return fmt.Sprintf("at most %d images per message", maxImages)
	}
	for i, img := range images {
		if !imageMimes[strings.ToLower(img.Mime)] {
			return fmt.Sprintf("image %d: unsupported type %q (want png, jpeg, gif or webp)", i+1, img.Mime)
		}
		raw, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return fmt.Sprintf("image %d: data is not base64", i+1)
		}
		if len(raw) == 0 {
			return fmt.Sprintf("image %d: empty", i+1)
		}
		if len(raw) > maxImageBytes {
			return fmt.Sprintf("image %d: larger than %d MB", i+1, maxImageBytes>>20)
		}
	}
	return ""
}

// imageBackend is the optional Backend seam for prompts with attachments.
type imageBackend interface {
	OnPromptImages(sessionID, text string, images []Image, appendEvent func(kind, text, status string))
}

// promptBackend runs one turn. Images need an imageBackend; a Backend
// without one refuses them in the transcript instead of dropping them.
func promptBackend(be Backend, id, text string, images []Image, appendEvent func(kind, text, status string)) {
	if len(images) == 0 {
		be.OnPrompt(id, text, appendEvent)
		return
	}
	if ib, ok := be.(imageBackend); ok {
		ib.OnPromptImages(id, text, images, appendEvent)
		return
	}
	appendEvent("error", "this harness cannot take images", "")
	appendEvent("status", "", "failed")
}
