package web

import (
	"bufio"
	"strings"
	"testing"
)

// A resume replay line past 1 MiB must not read as a closed adapter.
func TestACPReadLongLine(t *testing.T) {
	t.Parallel()
	c := &acpProcessConn{wait: map[int64]chan acpResult{}, readDone: make(chan struct{})}
	got := make(chan acpResult, 1)
	c.wait[1] = got
	long := `{"jsonrpc":"2.0","method":"x","params":{"d":"` + strings.Repeat("a", 3<<20) + `"}}`
	in := long + "\n" + `{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"
	c.read(bufio.NewScanner(strings.NewReader(in)))
	r := <-got
	if r.err != nil {
		t.Fatalf("call after long line failed: %v", r.err)
	}
}
