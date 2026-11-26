package session

// oscFilter replaces non-ASCII bytes inside OSC strings (window titles, links) before the emulator
// sees them: x/ansi ends the string at byte 0x9C even inside a UTF-8 character, and prints the rest
// of the title on the screen. nakama shows neither titles nor links, so nothing is lost.
type oscFilter struct{ state int }

const (
	oscNone   = iota
	oscEsc    // after ESC
	oscInside // inside ESC ] ... until BEL or ESC \
)

// clean rewrites p in place and returns it. The state carries over to the next read.
func (f *oscFilter) clean(p []byte) []byte {
	for i, c := range p {
		switch {
		case c == 0x1b: // also starts the ESC \ that ends an OSC
			f.state = oscEsc
		case f.state == oscEsc && c == ']':
			f.state = oscInside
		case f.state == oscEsc:
			f.state = oscNone
		case f.state == oscInside && (c == 0x07 || c == 0x18 || c == 0x1a):
			f.state = oscNone
		case f.state == oscInside && c >= 0x80:
			p[i] = '?'
		}
	}
	return p
}
