package office

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/mattn/go-sixel"
	"golang.org/x/sys/unix"
)

type Layout struct {
	Scale      float64
	Col, Row   int // 1-based cell where the picture starts
	Cols, Rows int // cells the picture covers
	ImgW, ImgH int // picture size in pixels
}

const minScale = 0.2

// Fit places a picture of bgW x bgH pixels in an area of cols x rows cells of cw x ch pixels:
// never enlarged, aspect kept, centred. ok is false when the area is too small to be worth drawing.
func Fit(cols, rows, cw, ch, bgW, bgH int) (Layout, bool) {
	return FitMax(cols, rows, cw, ch, bgW, bgH, 1)
}

// FitMax is Fit with an upper limit on the scale (0 or less means 1): a smaller picture is cheaper to
// encode and send every frame.
func FitMax(cols, rows, cw, ch, bgW, bgH int, maxScale float64) (Layout, bool) {
	if maxScale <= 0 || maxScale > 1 {
		maxScale = 1
	}
	if cols <= 0 || rows <= 0 || cw <= 0 || ch <= 0 || bgW <= 0 || bgH <= 0 {
		return Layout{}, false
	}
	scale := math.Min(maxScale, math.Min(float64(cols*cw)/float64(bgW), float64(rows*ch)/float64(bgH)))
	if scale < minScale {
		return Layout{}, false
	}
	l := Layout{Scale: scale, ImgW: int(math.Round(float64(bgW) * scale)), ImgH: int(math.Round(float64(bgH) * scale))}
	l.Cols, l.Rows = (l.ImgW+cw-1)/cw, (l.ImgH+ch-1)/ch
	l.Col, l.Row = 1+(cols-l.Cols)/2, 1+(rows-l.Rows)/2
	return l, true
}

// CellSize is the size of one terminal cell in pixels: from the terminal itself when it
// reports it, else from a "WxH" flag, else 8x16 (a guess on the small side: too small only makes the picture smaller, too big would run it off the bottom).
func CellSize(fd int, flag string) (cw, ch int, known bool) {
	if ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err == nil && ws.Xpixel > 0 && ws.Ypixel > 0 && ws.Col > 0 && ws.Row > 0 {
		return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row), true
	}
	var w, h int
	if _, err := fmt.Sscanf(flag, "%dx%d", &w, &h); err == nil && w > 0 && h > 0 {
		return w, h, true
	}
	return 8, 16, false
}

// ParseDA1 reports whether a primary device attributes reply (ESC [ ? ... c) lists sixel (attribute 4).
func ParseDA1(resp []byte) bool {
	s := string(resp)
	i := strings.Index(s, "\x1b[?")
	if i < 0 {
		return false
	}
	s = s[i+3:]
	j := strings.IndexByte(s, 'c')
	if j < 0 {
		return false
	}
	for _, p := range strings.Split(s[:j], ";") {
		if p == "4" {
			return true
		}
	}
	return false
}

// Encode turns a picture into a sixel sequence.
func Encode(img image.Image) ([]byte, error) { return encode(img, 64) }

func encode(img image.Image, colors int) ([]byte, error) {
	var buf bytes.Buffer
	e := sixel.NewEncoder(&buf)
	e.Dither = false
	e.Colors = colors
	if err := e.Encode(img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
