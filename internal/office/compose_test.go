package office

import (
	"image"
	"image/color"
	"testing"

	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/sprites"
	"github.com/kalyskhairy/nakama/internal/theme"
)

var (
	red    = color.NRGBA{200, 40, 40, 255}
	blue   = color.NRGBA{40, 40, 200, 255}
	orange = color.NRGBA{255, 165, 0, 255}
	yellow = color.NRGBA{255, 220, 0, 255}
)

func box(w, h int, c color.NRGBA) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	return im
}

// writeChar saves a synthetic character: walking frames w x 20, sleeping frames 2w x 10, working frames w+2 x 20.
func writeChar(t *testing.T, dir string, c color.NRGBA, w int) {
	t.Helper()
	s := &sprites.Sheet{}
	for ri := 0; ri < 4; ri++ {
		for i := 0; i < 4; i++ {
			s.Rows[ri] = append(s.Rows[ri], box(w, 20, c))
		}
	}
	for i := 0; i < 2; i++ {
		s.Rows[4] = append(s.Rows[4], box(2*w, 10, c))
	}
	for i := 0; i < 4; i++ {
		s.Rows[5] = append(s.Rows[5], box(w+2, 20, c))
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
}

func testRenderer(t *testing.T) *Renderer {
	t.Helper()
	root := t.TempDir()
	writeChar(t, root+"/c", red, 10)
	writeChar(t, root+"/d", blue, 10)
	th := &theme.Theme{
		Background: box(200, 100, color.NRGBA{90, 90, 90, 255}),
		CharHeight: 0.2, // 20 px on a 100 px background: sprites keep their size at scale 1
		Characters: map[string]string{"c": root + "/c", "d": root + "/d"},
		AgentTypes: map[string]string{"default": "c"},
	}
	r, err := NewRenderer(th)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func placed(id, ch string, x, y float64) scene.Placed {
	return scene.Placed{ID: id, Character: ch, P: theme.Point{X: x, Y: y}}
}

func count(im *image.NRGBA, r image.Rectangle, c color.NRGBA) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if image.Pt(x, y).In(im.Bounds()) && im.NRGBAAt(x, y) == c {
				n++
			}
		}
	}
	return n
}

func TestComposeSizeFollowsScale(t *testing.T) {
	r := testRenderer(t)
	for scale, want := range map[float64]image.Point{1: {200, 100}, 0.5: {100, 50}} {
		if b := r.Compose(nil, "", scale).Bounds(); b.Dx() != want.X || b.Dy() != want.Y {
			t.Errorf("scale %v: %v", scale, b)
		}
	}
}

func TestSpriteFeetAtPoint(t *testing.T) {
	r := testRenderer(t)
	im := r.Compose([]scene.Placed{placed("a", "c", 0.5, 0.8)}, "", 1)
	if im.NRGBAAt(100, 79) != red || im.NRGBAAt(95, 70) != red || im.NRGBAAt(104, 60) != red {
		t.Fatal("sprite is not standing on the point")
	}
	if im.NRGBAAt(100, 81) == red || im.NRGBAAt(100, 59) == red || im.NRGBAAt(94, 70) == red {
		t.Fatal("sprite spills outside its feet-anchored box")
	}
}

func TestLowerActorDrawnOver(t *testing.T) {
	r := testRenderer(t)
	// the lower actor (y 0.7, blue) is listed first on purpose: drawing order must follow y, not the slice
	im := r.Compose([]scene.Placed{placed("low", "d", 0.5, 0.7), placed("high", "c", 0.5, 0.6)}, "", 1)
	if got := im.NRGBAAt(100, 62); got != blue {
		t.Fatalf("overlap pixel %v, want blue (lower actor on top)", got)
	}
	if got := im.NRGBAAt(100, 45); got != red {
		t.Fatalf("pixel only the upper actor covers: %v", got)
	}
}

func TestMarksDrawn(t *testing.T) {
	r := testRenderer(t)
	above := image.Rect(80, 20, 120, 58) // the strip above a sprite standing at y 0.8 (top at 60)
	with := placed("a", "c", 0.5, 0.8)
	with.Mark = '?'
	if n := count(r.Compose([]scene.Placed{with}, "", 1), above, orange); n == 0 {
		t.Fatal("no mark pixels above the head")
	}
	if n := count(r.Compose([]scene.Placed{placed("a", "c", 0.5, 0.8)}, "", 1), above, orange); n != 0 {
		t.Fatalf("%d mark pixels without a mark", n)
	}
}

func TestSelectedActorIsMarked(t *testing.T) {
	r := testRenderer(t)
	ps := []scene.Placed{placed("a", "c", 0.5, 0.8)}
	whole := image.Rect(0, 0, 200, 100)
	if count(r.Compose(ps, "a", 1), whole, yellow) == 0 {
		t.Fatal("selected actor has no marker")
	}
	if n := count(r.Compose(ps, "other", 1), whole, yellow); n != 0 {
		t.Fatalf("%d marker pixels for an unselected actor", n)
	}
}

func TestDimDarkensSprite(t *testing.T) {
	r := testRenderer(t)
	normal := r.Compose([]scene.Placed{placed("a", "c", 0.5, 0.8)}, "", 1).NRGBAAt(100, 70)
	d := placed("a", "c", 0.5, 0.8)
	d.Dim = true
	dim := r.Compose([]scene.Placed{d}, "", 1).NRGBAAt(100, 70)
	if !(dim.R < normal.R && dim.G <= normal.G && dim.B <= normal.B) {
		t.Fatalf("normal %v dim %v", normal, dim)
	}
}

func TestFramesPerAnimation(t *testing.T) {
	r := testRenderer(t)
	sleep := placed("a", "c", 0.5, 0.8)
	sleep.Anim = scene.Sleep
	im := r.Compose([]scene.Placed{sleep}, "", 1)
	if im.NRGBAAt(91, 75) != red || im.NRGBAAt(100, 69) == red { // 20 wide x 10 tall, feet at 80
		t.Fatal("sleeping sprite has the wrong frame or size")
	}
	work := placed("a", "c", 0.5, 0.8)
	work.Anim = scene.Work
	if r.Compose([]scene.Placed{work}, "", 1).NRGBAAt(94, 70) != red { // 12 wide: x 94..105
		t.Fatal("work frame not used")
	}
}

func TestFrameIndexWrapsAndMissingCharacterIsSkipped(t *testing.T) {
	r := testRenderer(t)
	w := placed("a", "c", 0.5, 0.8)
	w.Anim, w.Frame, w.Facing = scene.Walk, 99, 2
	g := placed("g", "ghost", 0.2, 0.5)
	g.Anim, g.Frame, g.Facing = scene.Walk, -3, 9
	im := r.Compose([]scene.Placed{w, g}, "", 1)
	if im.NRGBAAt(100, 70) != red {
		t.Fatal("wrapped frame not drawn")
	}
	_ = r.Compose([]scene.Placed{placed("x", "c", -1, 2), placed("y", "c", 5, -5)}, "", 0.3) // off-screen points must not panic
}

func TestSubagentIsDrawnAsAShadow(t *testing.T) {
	r := testRenderer(t)
	lead := placed("a", "c", 0.25, 0.8)
	sub := placed("b", "c", 0.75, 0.8)
	sub.Sub = true
	im := r.Compose([]scene.Placed{lead, sub}, "", 1)
	l, s := im.NRGBAAt(50, 70), im.NRGBAAt(150, 70)
	if l != red {
		t.Fatalf("the lead keeps its colours: %v", l)
	}
	if int(s.R)+int(s.G)+int(s.B) >= int(l.R)+int(l.G)+int(l.B) || s.B <= l.B || s.A != 255 {
		t.Fatalf("a subagent must be darker and bluer than a lead: %v vs %v", s, l)
	}
	if int(s.R) < int(l.R)/3 {
		t.Fatalf("but light enough to see what it does: %v", s)
	}
}
