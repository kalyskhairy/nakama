package session

import (
	"strings"
	"testing"
)

// The emulator takes byte 0x9C inside a title for its end, even in the middle of "✳" (E2 9C B3),
// and printed the rest of the title where the cursor was: in Claude's input box.
func TestTitleWithSymbolIsNotPrinted(t *testing.T) {
	s := start(t, nil, "sh", "-c", `printf '> \033]0;\342\234\263 Judul sesi\007ok \342\234\263'; sleep 5`)
	waitFor(t, "output", func() bool { return strings.Contains(s.Text(), "ok") })
	if got := strings.TrimSpace(s.Text()); got != "> ok ✳" {
		t.Fatalf("screen %q: the title leaked, or text outside it was damaged", got)
	}
}

func TestOSCFilterWorksAcrossReads(t *testing.T) {
	var f oscFilter
	in := "a✳\x1b]0;✳ x\x1b\\b✳\x1b]2;Ü\x07Ü\x1b[1mÜ"
	var out []byte
	for i := 0; i < len(in); i++ { // one byte per read
		out = append(out, f.clean([]byte{in[i]})...)
	}
	if want := "a✳\x1b]0;??? x\x1b\\b✳\x1b]2;??\x07Ü\x1b[1mÜ"; string(out) != want {
		t.Fatalf("%q\nwant %q", out, want)
	}
}
