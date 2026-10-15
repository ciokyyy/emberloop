// Embedding bake-off for Memory v2 ticket 04. Usage: go run . /path/eval.json model...
// Tasks: (1) retrieval: raw user message -> its stored Memory text among all 15;
// (2) paraphrase vs contradiction cosine gap for the 0.88/0.95 dedup thresholds.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

var key string

func embed(model string, in []string) ([][]float64, float64, error) {
	b, _ := json.Marshal(map[string]any{"model": model, "input": in})
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/embeddings", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	t := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var r struct {
		Data []struct {
			Embedding []float64
			Index     int
		}
		Error *struct{ Message string }
		Usage struct{ Cost float64 }
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, 0, err
	}
	if r.Error != nil || len(r.Data) != len(in) {
		msg := "bad response"
		if r.Error != nil {
			msg = r.Error.Message
		}
		return nil, 0, fmt.Errorf("%s (http %d)", msg, resp.StatusCode)
	}
	out := make([][]float64, len(in))
	for _, d := range r.Data {
		out[d.Index] = d.Embedding
	}
	return out, time.Since(t).Seconds(), nil
}

func cos(a, b []float64) float64 {
	var d, x, y float64
	for i := range a {
		d += a[i] * b[i]
		x += a[i] * a[i]
		y += b[i] * b[i]
	}
	return d / (math.Sqrt(x) * math.Sqrt(y))
}

var para = [][2]string{
	{"Always return wrapped errors.", "Errors must always be returned wrapped."},
	{"The user prefers short, terse replies.", "Keep answers brief and to the point; the user dislikes long replies."},
	{"Run the repo check before committing.", "Before every commit, run the repository check."},
	{"Use Go for the backend.", "The backend is written in Go."},
	{"Memories should be saved automatically.", "Save memories automatically without asking."},
	{"Never use sudo for file edits.", "Do not use sudo when editing files."},
}
var contra = [][2]string{
	{"Always return wrapped errors.", "Never return wrapped errors."},
	{"The user prefers short, terse replies.", "The user prefers long, detailed replies."},
	{"Run the repo check before committing.", "Skip the repo check before committing."},
	{"Use Go for the backend.", "Do not use Go for the backend."},
	{"Memories should require human approval.", "Memories should not require human approval."},
	{"Always use sudo for file edits.", "Never use sudo for file edits."},
}
var unrelated = [][2]string{
	{"Always return wrapped errors.", "The user prefers dark mode in the editor."},
	{"Use Go for the backend.", "Deploy to the VPS on Fridays."},
	{"Run the repo check before committing.", "The quota card shows percent first."},
}

func main() {
	o, _ := exec.Command("pig", "auth", "print-api-key", "--provider", "openrouter").Output()
	key = strings.TrimSpace(string(o))
	var ev struct {
		Pos       [][2]string `json:"Pos"`
		Positives []struct {
			Text    string `json:"text"`
			Concept string `json:"concept"`
		} `json:"positives"`
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read eval:", err)
		os.Exit(1)
	}
	if err := json.Unmarshal(b, &ev); err != nil {
		fmt.Fprintln(os.Stderr, "parse eval:", err)
		os.Exit(1)
	}
	var msgs, mems []string
	for _, p := range ev.Pos {
		msgs = append(msgs, p[0])
		mems = append(mems, p[1])
	}
	for _, p := range ev.Positives {
		msgs = append(msgs, p.Text)
		mems = append(mems, p.Concept)
	}
	if len(msgs) == 0 {
		fmt.Fprintln(os.Stderr, "eval contains no labelled positive message/concept pairs")
		os.Exit(1)
	}
	fmt.Printf("%-40s %5s %5s %5s %7s | %6s %6s %6s %6s | %5s\n", "model", "dims", "top1", "top3", "MRR", "para", "contra", "unrel", "gap", "sec")
	for _, m := range os.Args[2:] {
		var all []string
		all = append(all, msgs...)
		all = append(all, mems...)
		for _, set := range [][][2]string{para, contra, unrelated} {
			for _, p := range set {
				all = append(all, p[0], p[1])
			}
		}
		vs, sec, err := embed(m, all)
		if err != nil {
			fmt.Printf("%-40s ERR %v\n", m, err)
			continue
		}
		n := len(msgs)
		q, d := vs[:n], vs[n:2*n]
		var t1, t3 int
		var mrr float64
		for i := range q {
			type sc struct {
				j int
				s float64
			}
			var l []sc
			for j := range d {
				l = append(l, sc{j, cos(q[i], d[j])})
			}
			sort.Slice(l, func(a, b int) bool { return l[a].s > l[b].s })
			for r, x := range l {
				if x.j == i {
					if r == 0 {
						t1++
					}
					if r < 3 {
						t3++
					}
					mrr += 1 / float64(r+1)
				}
			}
		}
		off := 2 * n
		avg := func(set [][2]string) float64 {
			var s float64
			for i := range set {
				s += cos(vs[off+2*i], vs[off+2*i+1])
			}
			off += 2 * len(set)
			return s / float64(len(set))
		}
		pa, co, un := avg(para), avg(contra), avg(unrelated)
		fmt.Printf("%-40s %5d %2d/%-2d %2d/%-2d %7.3f | %6.3f %6.3f %6.3f %6.3f | %5.1f\n", m, len(vs[0]), t1, n, t3, n, mrr/float64(n), pa, co, un, pa-co, sec)
	}
}
