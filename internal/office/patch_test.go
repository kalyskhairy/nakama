package office

import (
	"image"
	"image/color"
	"reflect"
	"testing"

	"github.com/kalyskhairy/nakama/internal/scene"
)

func TestDirtyCoversOnlyWhatChanged(t *testing.T) {
	a := box(100, 60, color.NRGBA{1, 1, 1, 255})
	b := box(100, 60, color.NRGBA{1, 1, 1, 255})
	if got := Dirty(a, b, 20, 12); len(got) != 0 {
		t.Fatalf("same picture: %v", got)
	}
	for _, p := range []image.Point{{25, 14}, {45, 14}, {25, 26}, {45, 26}, {99, 59}} {
		b.SetNRGBA(p.X, p.Y, color.NRGBA{9, 9, 9, 255})
	}
	want := []image.Rectangle{image.Rect(20, 12, 60, 36), image.Rect(80, 48, 100, 60)}
	if got := Dirty(a, b, 20, 12); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPartIsEncodedInTheSharedColours(t *testing.T) {
	r := testRenderer(t)
	pal, err := r.Palette()
	if err != nil || len(pal) == 0 {
		t.Fatalf("palette %d %v", len(pal), err)
	}
	im := r.Compose([]scene.Placed{placed("a", "c", 0.5, 0.5)}, "", 1)
	out, err := EncodePart(im, image.Rect(80, 24, 120, 60), pal)
	if err != nil || len(out) == 0 {
		t.Fatalf("%d bytes %v", len(out), err)
	}
}

func TestASmallColourOfACharacterKeepsItsPlaceInThePalette(t *testing.T) {
	r := testRenderer(t)
	bg := image.NewNRGBA(image.Rect(0, 0, 200, 100)) // hundreds of browns that would take every slot
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			bg.SetNRGBA(x, y, color.NRGBA{uint8(100 + x/2), uint8(50 + y), uint8(20 + (x+y)%40), 255})
		}
	}
	r.th.Background = bg
	lavender := color.NRGBA{180, 150, 230, 255}
	for _, f := range r.sheets["d"].Rows[0] {
		f.SetNRGBA(f.Bounds().Dx()/2, 2, lavender)
		f.SetNRGBA(f.Bounds().Dx()/2+1, 2, lavender)
	}
	pal, err := r.Palette()
	if err != nil {
		t.Fatal(err)
	}
	best := 1 << 30
	for _, c := range pal {
		k := color.NRGBAModel.Convert(c).(color.NRGBA)
		d := (int(k.R)-180)*(int(k.R)-180) + (int(k.G)-150)*(int(k.G)-150) + (int(k.B)-230)*(int(k.B)-230)
		best = min(best, d)
	}
	if best > 20*20 || len(pal) > 255 {
		t.Fatalf("nearest colour is %d away; %d colours", best, len(pal))
	}
}
