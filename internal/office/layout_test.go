package office

import (
	"bytes"
	"image"
	"math"
	"testing"
)

func TestFitNeverUpscalesAndKeepsAspect(t *testing.T) {
	l, ok := Fit(200, 60, 10, 20, 1536, 730)
	if !ok || l.Scale != 1 || l.ImgW != 1536 || l.ImgH != 730 {
		t.Fatalf("big area: %+v ok=%v", l, ok)
	}
	l, ok = Fit(100, 40, 10, 20, 1536, 730)
	if !ok || l.Scale >= 1 {
		t.Fatalf("narrow area: %+v ok=%v", l, ok)
	}
	if got, want := float64(l.ImgW)/float64(l.ImgH), 1536.0/730.0; math.Abs(got-want) > 0.02 {
		t.Fatalf("aspect %.3f, want %.3f", got, want)
	}
	if l.ImgW > 100*10 || l.ImgH > 40*20 {
		t.Fatalf("image %dx%d overflows the 1000x800 area", l.ImgW, l.ImgH)
	}
}

func TestFitCenters(t *testing.T) {
	l, ok := Fit(200, 60, 10, 20, 1536, 730) // 154 x 37 cells
	if !ok || l.Cols != 154 || l.Rows != 37 {
		t.Fatalf("%+v", l)
	}
	if l.Col != 1+(200-154)/2 || l.Row != 1+(60-37)/2 {
		t.Fatalf("not centered: col %d row %d", l.Col, l.Row)
	}
}

func TestFitTooSmallReturnsNotOK(t *testing.T) {
	for _, a := range [][4]int{{5, 3, 10, 20}, {0, 10, 10, 20}, {10, 0, 10, 20}, {10, 10, 0, 20}, {-1, -1, 10, 20}} {
		if _, ok := Fit(a[0], a[1], a[2], a[3], 1536, 730); ok {
			t.Errorf("%v should not fit", a)
		}
	}
	if _, ok := Fit(100, 40, 10, 20, 0, 730); ok {
		t.Error("zero-width background should not fit")
	}
}

func TestCellSizeFallbacks(t *testing.T) {
	cases := map[string][2]int{"12x24": {12, 24}, "": {8, 16}, "junk": {8, 16}, "0x0": {8, 16}, "-4x8": {8, 16}}
	for flag, want := range cases {
		if cw, ch, _ := CellSize(-1, flag); cw != want[0] || ch != want[1] { // fd -1: the ioctl cannot answer
			t.Errorf("flag %q: %dx%d, want %v", flag, cw, ch, want)
		}
	}
}

func TestParseDA1(t *testing.T) {
	for in, want := range map[string]bool{
		"\x1b[?62;4;22c": true, "\x1b[?4c": true, "\x1b[?62;22c": false, "\x1b[?64;14;24c": false,
		"garbage": false, "": false, "\x1b[?62;41c": false,
	} {
		if got := ParseDA1([]byte(in)); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}

func TestEncodeProducesSixel(t *testing.T) {
	out, err := Encode(box(8, 8, red))
	if err != nil || !bytes.HasPrefix(out, []byte("\x1bP")) || !bytes.HasSuffix(out, []byte("\x1b\\")) {
		t.Fatalf("err=%v out=%q", err, out)
	}
	if _, err := Encode(image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("1x1: %v", err)
	}
}

func TestFitMaxCapsTheScale(t *testing.T) {
	l, ok := FitMax(200, 60, 10, 20, 1536, 730, 0.6) // plenty of room: the cap decides
	if !ok || l.Scale != 0.6 || l.ImgW != 922 {
		t.Fatalf("%+v ok=%v", l, ok)
	}
	l, ok = FitMax(60, 20, 10, 20, 1536, 730, 0.6) // little room: the room decides
	if !ok || l.Scale >= 0.6 {
		t.Fatalf("%+v ok=%v", l, ok)
	}
	if l, _ := FitMax(200, 60, 10, 20, 1536, 730, 0); l.Scale != 1 {
		t.Fatalf("a cap of 0 means no cap: %+v", l)
	}
	if l, _ := Fit(200, 60, 10, 20, 1536, 730); l.Scale != 1 {
		t.Fatalf("Fit must stay uncapped: %+v", l)
	}
}
