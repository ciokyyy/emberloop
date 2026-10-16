package web

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalBootPayloadEscapesScriptContent(t *testing.T) {
	boot := map[string]json.RawMessage{
		"/api/session": json.RawMessage(`{"title":"</script><script>alert(1)</script>","separator":"\u2028"}`),
	}
	got := string(marshalBootPayload(boot))
	if strings.Contains(got, "<") || strings.Contains(got, "\u2028") {
		t.Fatalf("boot payload contains script-unsafe characters: %s", got)
	}
	if !strings.Contains(got, `\u003c/script\u003e`) || !strings.Contains(got, `\u2028`) {
		t.Fatalf("boot payload did not preserve escaped JSON values: %s", got)
	}
}
