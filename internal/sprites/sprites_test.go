package sprites

import (
	"image"
	"image/color"
	"testing"
)

var magenta = color.NRGBA{255, 0, 255, 255}

func fill(im *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
}

// testSheet draws a 1024x1536 sheet like the ChatGPT ones: 6 rows, 4/4/4/4/2/4 frames,
// every frame a body box with a yellow "hat" on its LEFT side (so a mirror is detectable).
func testSheet() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, 1024, 1536))
	fill(im, im.Bounds(), magenta)
	rows := []struct{ y0, h, n int }{{20, 200, 4}, {260, 200, 4}, {500, 200, 4}, {740, 200, 4}, {1020, 100, 2}, {1210, 200, 4}}
	for _, r := range rows {
		for i := 0; i < r.n; i++ {
			x0 := 60 + i*256
			fill(im, image.Rect(x0, r.y0, x0+100, r.y0+r.h), color.NRGBA{200, 40, 40, 255})
			fill(im, image.Rect(x0, r.y0, x0+30, r.y0+30), color.NRGBA{250, 220, 0, 255}) // left-side marker
		}
	}
	return im
}

func counts(s *Sheet) []int {
	out := make([]int, 6)
	for i, r := range s.Rows {
		out[i] = len(r)
	}
	return out
}

func TestSliceFindsSixRowsAndFrames(t *testing.T) {
	s, err := Slice(testSheet(), 64)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{4, 4, 4, 4, 2, 4}
	got := counts(s)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frames per row %v, want %v", got, want)
		}
	}
}

func TestSliceRemovesMagentaAndFringe(t *testing.T) {
	im := testSheet()
	im.SetNRGBA(500, 480, color.NRGBA{120, 120, 120, 255}) // a lone speck between two rows
	im.SetNRGBA(10, 10, color.NRGBA{240, 10, 250, 255})    // off-magenta noise
	s, err := Slice(im, 64)
	if err != nil {
		t.Fatal(err)
	}
	if got := counts(s); got[1] != 4 || got[2] != 4 {
		t.Fatalf("specks created extra frames: %v", got)
	}
	for _, row := range s.Rows {
		for _, f := range row {
			for y := 0; y < f.Bounds().Dy(); y++ {
				for x := 0; x < f.Bounds().Dx(); x++ {
					c := f.NRGBAAt(x, y)
					if c.A > 0 && int(min(c.R, c.B))-int(c.G) > 110 {
						t.Fatalf("magenta left at %d,%d: %v", x, y, c)
					}
				}
			}
		}
	}
}

func TestLeftRowIsMirrorOfRight(t *testing.T) {
	s, err := Slice(testSheet(), 64)
	if err != nil {
		t.Fatal(err)
	}
	for i := range s.Rows[2] {
		r, l := s.Rows[2][i], s.Rows[1][i]
		if r.Bounds() != l.Bounds() {
			t.Fatalf("frame %d sizes differ: %v %v", i, r.Bounds(), l.Bounds())
		}
		w := r.Bounds().Dx()
		for y := 0; y < r.Bounds().Dy(); y++ {
			for x := 0; x < w; x++ {
				if r.NRGBAAt(x, y) != l.NRGBAAt(w-1-x, y) {
					t.Fatalf("frame %d differs at %d,%d", i, x, y)
				}
			}
		}
	}
}

func TestHeightsNormalized(t *testing.T) {
	s, err := Slice(testSheet(), 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, ri := range []int{0, 1, 2, 3, 5} {
		for i, f := range s.Rows[ri] {
			if f.Bounds().Dy() != 64 {
				t.Errorf("row %d frame %d height %d, want 64", ri, i, f.Bounds().Dy())
			}
		}
	}
	for i, f := range s.Rows[4] { // sleeping: same scale, so shorter
		if h := f.Bounds().Dy(); h >= 64 || h < 25 {
			t.Errorf("sleep frame %d height %d, want about 32", i, h)
		}
	}
}

func TestSliceErrorsOnWrongLayout(t *testing.T) {
	empty := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	fill(empty, empty.Bounds(), magenta)
	if _, err := Slice(empty, 64); err == nil {
		t.Error("blank sheet should fail")
	}
	im := testSheet()
	fill(im, image.Rect(0, 1200, 1024, 1536), magenta) // erase the last row: 5 rows left
	if _, err := Slice(im, 64); err == nil {
		t.Error("5 rows should fail")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s, err := Slice(testSheet(), 64)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, w := counts(l), counts(s)
	for i := range w {
		if g[i] != w[i] {
			t.Fatalf("counts %v vs %v", g, w)
		}
	}
	if l.Rows[0][0].NRGBAAt(30, 30) != s.Rows[0][0].NRGBAAt(30, 30) {
		t.Fatal("pixels differ after round trip")
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("empty directory should not load")
	}
}
