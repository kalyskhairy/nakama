package theme

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestPortraitTurnsTheRightEndUp(t *testing.T) {
	bg := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	red := color.NRGBA{255, 0, 0, 255}
	bg.SetNRGBA(399, 100, red) // the bow, at the right end
	in := &Theme{Background: bg, CharHeight: 0.1, Nodes: map[string]Point{"bow": {X: 1, Y: 0.25}},
		Stations: []Station{{ID: "s", Point: Point{X: 0.75, Y: 0.5}}}}
	p := Portrait(in)
	if b := p.Background.Bounds(); b.Dx() != 200 || b.Dy() != 400 {
		t.Fatalf("size %v", b)
	}
	if got := color.NRGBAModel.Convert(p.Background.At(100, 0)); got != red {
		t.Fatalf("the right end must be at the top, got %v", got)
	}
	if n := p.Nodes["bow"]; n != (Point{X: 0.25, Y: 0}) {
		t.Fatalf("node %+v", n)
	}
	if s := p.Stations[0]; s.Point != (Point{X: 0.5, Y: 0.25}) || s.ID != "s" {
		t.Fatalf("station %+v", s)
	}
	if math.Abs(p.CharHeight-0.05) > 1e-9 { // 0.1 of 200 px = 20 px = 0.05 of 400 px
		t.Fatalf("a character must keep its size on the ship: %v", p.CharHeight)
	}
	if in.Nodes["bow"] != (Point{X: 1, Y: 0.25}) || in.Stations[0].Point != (Point{X: 0.75, Y: 0.5}) {
		t.Fatal("Portrait changed its input")
	}
}
