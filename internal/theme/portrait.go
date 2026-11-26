package theme

import "image"

// Portrait is the theme turned a quarter counter-clockwise, so the right end of the background (the
// bow) points up. Characters stay upright: only the background and the coordinates turn.
func Portrait(t *Theme) *Theme {
	b := t.Background.Bounds()
	w, h := b.Dx(), b.Dy()
	bg := image.NewNRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bg.Set(y, w-1-x, t.Background.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	turn := func(p Point) Point { return Point{X: p.Y, Y: 1 - p.X} }
	out := *t
	out.Background = bg
	out.CharHeight = t.CharHeight * float64(h) / float64(w) // same pixels, the picture is now w high
	out.Nodes = make(map[string]Point, len(t.Nodes))
	for id, p := range t.Nodes {
		out.Nodes[id] = turn(p)
	}
	out.Stations = append([]Station(nil), t.Stations...)
	for i := range out.Stations {
		out.Stations[i].Point = turn(out.Stations[i].Point)
	}
	return &out
}
