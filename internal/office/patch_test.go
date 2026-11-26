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
