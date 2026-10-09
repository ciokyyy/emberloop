package el

// Row macros (input-queue ticket 03): the input row's own commands. A
// submitted row text of the form "/name args..." first checks el's
// built-ins (/compress calls the existing compress request for the wrapped
// session, whatever the harness), then the prompt macros in settings (the
// {{args}} template expands and enqueues like typed text, through the same
// queue and hold rules). Any other /x text is a normal message.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"example.com/emberloop/tools"
)

// submitRowText enqueues one submitted row text with its kind (steer or
// queue): a built-in runs, a prompt macro expands and enqueues, anything
// else enqueues as typed. The prompt holds the wrapper lock; the compress
// request runs in the background, since it waits on a tool-call boundary.
func (w *wrapper) submitRowText(kind QueueKind, text string) {
	if name, args, ok := tools.ParseMacroCall(text); ok {
		if tools.IsBuiltinMacro(name) {
			go w.runRowCompress()
			return
		}
		if m, ok := w.cfg.Settings.LookupMacro(name); ok {
			if expanded := tools.ExpandMacro(m.Template, args); strings.TrimSpace(expanded) != "" {
				w.queue.enqueue(kind, expanded, "user")
				return
			}
		}
	}
	w.queue.enqueue(kind, text, "user")
}

// runRowCompress is the /compress built-in: the existing compress request
// for the wrapped session's own pane, through the wrapper's Exec (the real
// one in production, the fake in tests). Without a recorded pane there is
// nothing to compress in, so it warns on the wrapper's error sink and runs
// nothing.
func (w *wrapper) runRowCompress() {
	w.rowCompressMu.Lock()
	if w.rowCompressing {
		w.rowCompressMu.Unlock()
		warnRowCompress(w.cfg.Stderr, fmt.Errorf("compress already running"))
		return
	}
	w.rowCompressing = true
	w.rowCompressMu.Unlock()
	defer func() {
		w.rowCompressMu.Lock()
		w.rowCompressing = false
		w.rowCompressMu.Unlock()
	}()
	run := w.cfg.CompressRun
	if run == nil {
		run = OSExec
	}
	pane := ""
	if st, ok, err := ReadAgentState(w.cfg.Source.AgentID); err == nil && ok {
		pane = st.Pane
	}
	if pane == "" {
		warnRowCompress(w.cfg.Stderr, fmt.Errorf("no pane recorded for agent %q: run inside herdr", w.cfg.Source.AgentID))
		return
	}
	if _, err := RequestCompress(run, pane, w.cfg.Settings); err != nil {
		warnRowCompress(w.cfg.Stderr, err)
	}
}

// warnRowCompress reports a failed /compress on the wrapper's error sink,
// never into the child's terminal.
func warnRowCompress(sink io.Writer, err error) {
	if sink == nil {
		sink = os.Stderr
	}
	fmt.Fprintf(sink, "el: /compress: %v\n", err)
}

// reportMacroValidation reports duplicate and shadowing macros at settings
// load: rejects read as errors, built-in shadows as warnings. El's macro
// wins every collision, so the row runs with the valid entries only.
func reportMacroValidation(sink io.Writer, s tools.Settings) {
	if sink == nil {
		sink = os.Stderr
	}
	_, warnings, err := s.ValidateMacros()
	if err != nil {
		fmt.Fprintf(sink, "el: macros: %v\n", err)
	}
	for _, warning := range warnings {
		fmt.Fprintf(sink, "el: macros: %s\n", warning)
	}
}
