// Command fakellm: deterministic local fake LLM server for benchmarking
// coding-agent harnesses. Scripted replies, no real model calls.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	modelsFile := flag.String("models", "", "JSON model catalog file")
	logFile := flag.String("log", "", "JSONL request log file")
	latencyMs := flag.Int("latency-ms", 0, "base latency applied to every response (ms)")
	cooperate := flag.Bool("cooperate", false, "enable COOPERATE fuzzy tool-calling (also X-Fake-Cooperate header, FAKELLM_COOPERATE env)")
	faults := flag.String("faults", "", "failure-injection by-counter config: JSON ({\"3\":\"http500\"}) or file path")
	faultRate := flag.Float64("fault-rate", 0, "soak probability p per request of a random fault (0..1)")
	faultSeed := flag.Int64("fault-seed", 42, "seed for --fault-rate RNG")
	flag.Parse()

	models := defaultModels()
	if *modelsFile != "" {
		loaded, err := LoadModels(*modelsFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fakellm: %v\n", err)
			os.Exit(1)
		}
		if len(loaded) == 0 {
			fmt.Fprintln(os.Stderr, "fakellm: models file has no usable entries")
			os.Exit(1)
		}
		models = loaded
	}

	srv, err := NewServer(models, *logFile, time.Duration(*latencyMs)*time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakellm: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()
	if *cooperate {
		srv.SetCooperate(true)
	}
	if *faults != "" {
		m, err := parseFaultsConfig(*faults)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fakellm: %v\n", err)
			os.Exit(1)
		}
		srv.SetFaults(m)
	}
	if *faultRate < 0 || *faultRate > 1 {
		fmt.Fprintln(os.Stderr, "fakellm: --fault-rate must be 0..1")
		os.Exit(1)
	}
	if *faultRate > 0 {
		srv.SetFaultRate(*faultRate, *faultSeed)
	}

	log.Printf("fakellm: listening on http://%s with %d models", *addr, len(models))
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "fakellm: %v\n", err)
		os.Exit(1)
	}
}
