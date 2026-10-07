package emberloop

import "testing"

func TestNextKeyCanonical(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"\x03", "\x03"},
		{"\x1b[99;5u", "\x03"},
		{"\x1b[99;133u", "\x03"}, // ctrl + num lock
		{"\x1b[27;5;99~", "\x03"},
		{"\x1b[111;5u", "\x0f"},
		{"\x1b[13u", "\r"},
		{"\x1b[13;2u", "\n"},
		{"\x1b[13;3u", "\x1b\r"},
		{"\x1b[127;5u", "\x1b\x7f"},
		{"\x1b[127u", "\x7f"},
		{"\x1b[97u", "a"},
		{"\x1b[97;2u", "A"},
		{"\x1b[97:65;2u", "A"},
		{"\x1b[97;1:3u", ""},
		{"\x1b[I", ""},
		{"\x1b[O", ""},
		{"\x1b[1;5D", "\x1bb"},
		{"\x1b[1;5C", "\x1bf"},
		{"\x1b[1;2A", "\x1b[A"},
		{"\x1b[3;5~", "\x1bd"},
		{"\x1bOA", "\x1b[A"},
		{"\x1b[?62;c", ""},
		{"\x1b", "\x1b"},
		{"\x1bb", "\x1bb"},
		{"héllo", "héllo"},
	}
	for _, c := range cases {
		got, ok := singleKey([]byte(c.in))
		if !ok || got != c.want {
			t.Errorf("singleKey(%q) = %q, %v; want %q", c.in, got, ok, c.want)
		}
	}
}

func TestNextKeyIncomplete(t *testing.T) {
	for _, in := range []string{"\x1b[", "\x1b[99;5", "\x1bO"} {
		if _, _, complete := nextKey([]byte(in)); complete {
			t.Errorf("nextKey(%q) complete; want incomplete", in)
		}
	}
}

func TestRawHelpersSeeCSIu(t *testing.T) {
	if !isRawToggle([]byte("\x1b[111;5u")) {
		t.Error("Ctrl+O as CSI-u must toggle raw mode")
	}
	if !isRawReset([]byte("\x1b[99;5u")) || !isRawReset([]byte("\x1b[13u")) {
		t.Error("Ctrl+C and Enter as CSI-u must reset the dirty flag")
	}
	if isRawReset([]byte("\x1b[97u")) {
		t.Error("plain letter must not reset the dirty flag")
	}
}

func TestRowCtrlCAsCSIu(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("draft"))
	act, _ := row.handleInput([]byte("\x1b[99;5u"))
	if act != rowActionInterrupt {
		t.Fatalf("action = %v, want interrupt", act)
	}
	if row.draft() != "" {
		t.Fatalf("draft = %q, want empty", row.draft())
	}
}

func TestRowFocusNeverInserted(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("a\x1b[I\x1b[Ob"))
	if got := row.draft(); got != "ab" {
		t.Fatalf("draft = %q, want ab", got)
	}
}

func TestRowCtrlBackspaceDeletesWord(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("one two"))
	row.handleInput([]byte("\x1b[127;5u"))
	if got := row.draft(); got != "one " {
		t.Fatalf("draft = %q, want %q", got, "one ")
	}
}

func TestRowSplitSequenceAcrossReads(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("x"))
	if act, _ := row.handleInput([]byte("\x1b[99;")); act != rowActionNone {
		t.Fatalf("partial sequence acted: %v", act)
	}
	if act, _ := row.handleInput([]byte("5u")); act != rowActionInterrupt {
		t.Fatalf("completed sequence action = %v, want interrupt", act)
	}
}

func TestRowWordMotionAndCursorInsert(t *testing.T) {
	row := newInputRow()
	row.handleInput([]byte("one two"))
	row.handleInput([]byte("\x1b[1;5D")) // ctrl+left
	row.handleInput([]byte("X"))
	if got := row.draft(); got != "one Xtwo" {
		t.Fatalf("draft = %q, want %q", got, "one Xtwo")
	}
}
