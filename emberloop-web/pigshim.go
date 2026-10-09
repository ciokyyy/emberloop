package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PigShimMode is the first argument that runs the Daemon binary as the pi
// stand-in for pi-acp. pi-acp starts `pi --mode rpc`; the per-Session `pi`
// script runs this mode, which starts pig and relays its RPC streams byte for
// byte. It also listens on a unix socket so the Daemon can inject a native
// pig `steer` command into the running turn, which pi-acp cannot do: it only
// queues a second prompt.
const PigShimMode = "pig-shim"

// steerIDPrefix marks RPC requests the shim sends itself, so their
// responses never reach pi-acp, which does not know the ids.
const steerIDPrefix = "el-steer-"

// RunPigShim starts pig with args, relays stdio and serves steer requests on
// sock until pig exits. It returns pig's exit code.
func RunPigShim(pigBin string, args []string, sock string) int {
	cmd := exec.Command(pigBin, args...)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pig-shim:", err)
		return 1
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pig-shim:", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "pig-shim:", err)
		return 1
	}
	s := &pigShim{stdin: stdin, wait: map[string]chan json.RawMessage{}}
	if sock != "" {
		_ = os.Remove(sock)
		if ln, err := net.Listen("unix", sock); err == nil {
			defer os.Remove(sock)
			go s.serve(ln)
		}
	}
	// pi-acp's requests go to pig as whole lines under the same lock the
	// steer writer takes, so the two never interleave inside a line.
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
		for sc.Scan() {
			s.writeLine(sc.Bytes())
		}
		_ = stdin.Close()
	}()
	br := bufio.NewReaderSize(stdout, 1024*1024)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if !s.consume(line) {
				s.trackQueue(line)
				_, _ = os.Stdout.Write(line)
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, "pig-shim:", err)
			}
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}

type pigShim struct {
	mu    sync.Mutex
	stdin io.Writer
	wait  map[string]chan json.RawMessage
	next  int
	// queue is pig's last queue_update: the steers and follow-ups it holds.
	queue pigQueue
}

type pigQueue struct {
	Steering []string `json:"steering"`
	FollowUp []string `json:"followUp"`
}

// trackQueue keeps the latest queue_update pig printed.
func (s *pigShim) trackQueue(line []byte) {
	if !bytes.Contains(line, []byte(`"queue_update"`)) {
		return
	}
	var m struct {
		Type string `json:"type"`
		pigQueue
	}
	if json.Unmarshal(line, &m) != nil || m.Type != "queue_update" {
		return
	}
	s.mu.Lock()
	s.queue = m.pigQueue
	s.mu.Unlock()
}

func (s *pigShim) writeLine(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.stdin.Write(append(append([]byte(nil), b...), '\n'))
}

// consume swallows a response to one of the shim's own requests.
func (s *pigShim) consume(line []byte) bool {
	if !bytes.Contains(line, []byte(steerIDPrefix)) {
		return false
	}
	var m struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(line, &m) != nil || !strings.HasPrefix(m.ID, steerIDPrefix) {
		return false
	}
	s.mu.Lock()
	ch := s.wait[m.ID]
	delete(s.wait, m.ID)
	s.mu.Unlock()
	if ch != nil {
		ch <- append(json.RawMessage(nil), line...)
	}
	return true
}

// serve answers one JSON request per connection: {"message":"..."} is
// sent to pig as a steer command and the reply is pig's own response line.
func (s *pigShim) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *pigShim) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	var req struct {
		Op      string `json:"op"`
		Message string `json:"message"`
		// Images are pig RPC image blocks ({type,data,mimeType}).
		Images []map[string]string `json:"images"`
	}
	if err := json.NewDecoder(c).Decode(&req); err != nil {
		return
	}
	switch req.Op {
	case "queue":
		s.mu.Lock()
		q := s.queue
		s.mu.Unlock()
		_ = json.NewEncoder(c).Encode(map[string]any{"success": true, "data": q})
		return
	case "steer", "follow_up":
		if req.Message == "" && len(req.Images) == 0 {
			return
		}
	case "clear_queue":
	default:
		return
	}
	s.mu.Lock()
	s.next++
	id := fmt.Sprintf("%s%d", steerIDPrefix, s.next)
	ch := make(chan json.RawMessage, 1)
	s.wait[id] = ch
	s.mu.Unlock()
	cmd := map[string]any{"id": id, "type": req.Op}
	if req.Message != "" || len(req.Images) > 0 {
		cmd["message"] = req.Message
	}
	if len(req.Images) > 0 {
		cmd["images"] = req.Images
	}
	raw, _ := json.Marshal(cmd)
	s.writeLine(raw)
	select {
	case resp := <-ch:
		_, _ = c.Write(resp)
	case <-time.After(10 * time.Second):
		s.mu.Lock()
		delete(s.wait, id)
		s.mu.Unlock()
	}
}

// shimCall sends one request to a Session's shim and decodes pig's reply.
func shimCall(sock string, req map[string]any, out any) bool {
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(12 * time.Second))
	raw, _ := json.Marshal(req)
	if _, err := c.Write(raw); err != nil {
		return false
	}
	return json.NewDecoder(c).Decode(out) == nil
}

// shimQueueCmd sends steer or follow_up and reports whether pig queued it.
func shimQueueCmd(sock, op, text string, images []Image) bool {
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Disposition string `json:"disposition"`
		} `json:"data"`
	}
	req := map[string]any{"op": op, "message": text}
	if len(images) > 0 {
		blocks := make([]map[string]string, 0, len(images))
		for _, img := range images {
			blocks = append(blocks, map[string]string{"type": "image", "data": img.Data, "mimeType": strings.ToLower(img.Mime)})
		}
		req["images"] = blocks
	}
	if !shimCall(sock, req, &resp) {
		return false
	}
	return resp.Success && resp.Data.Disposition == "queued"
}
