package office

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"

	"github.com/mattn/go-sixel"

	"github.com/kalyskhairy/nakama/internal/sprites"
)

// Dirty lists the rectangles, on a grid of tileW x tileH, in which two pictures of one size differ.
// Tiles side by side are merged, and so are equal runs on rows that follow each other.
func Dirty(prev, cur *image.NRGBA, tileW, tileH int) []image.Rectangle {
	b := cur.Bounds()
	changed := func(r image.Rectangle) bool {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			i, j := cur.PixOffset(r.Min.X, y), cur.PixOffset(r.Max.X, y)
			if !bytes.Equal(prev.Pix[i:j], cur.Pix[i:j]) {
				return true
			}
		}
		return false
	}
	var out []image.Rectangle
	for y := b.Min.Y; y < b.Max.Y; y += tileH {
		start := -1
		for x := b.Min.X; ; x += tileW {
			hit := x < b.Max.X && changed(image.Rect(x, y, x+tileW, y+tileH).Intersect(b))
			if hit && start < 0 {
				start = x
			}
			if !hit && start >= 0 {
				r := image.Rect(start, y, x, y+tileH).Intersect(b)
				start = -1
				merged := false
				for i := range out {
					if o := out[i]; o.Min.X == r.Min.X && o.Max.X == r.Max.X && o.Max.Y == r.Min.Y {
						out[i].Max.Y, merged = r.Max.Y, true
					}
				}
				if !merged {
					out = append(out, r)
				}
			}
			if x >= b.Max.X {
				break
			}
		}
	}
	return out
}

// Palette is one set of colours for the whole office, so a patch matches the picture around it.
func (r *Renderer) Palette() (color.Palette, error) {
	if r.pal != nil {
		return r.pal, nil
	}
	b := r.th.Background.Bounds()
	sample := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(sample, sample.Bounds(), r.th.Background, b.Min, draw.Src)
	x := 0 // every character standing in a row along the bottom, so their colours are counted
	for _, s := range r.sheets {
		h := max(1, int(r.th.CharHeight*float64(b.Dy())))
		f := s.Rows[0][0]
		f = sprites.Resize(f, max(1, f.Bounds().Dx()*h/max(1, f.Bounds().Dy())), h)
		draw.Draw(sample, image.Rect(x, b.Dy()-h, x+f.Bounds().Dx(), b.Dy()), f, f.Bounds().Min, draw.Over)
		x += f.Bounds().Dx()
	}
	enc, err := encode(sample, 240) // many, or small red things turn brown
	if err != nil {
		return nil, err
	}
	var decoded image.Image
	if err := sixel.NewDecoder(bytes.NewReader(enc)).Decode(&decoded); err != nil {
		return nil, err
	}
	seen := map[color.NRGBA]bool{}
	d := decoded.Bounds()
	for y := d.Min.Y; y < d.Max.Y; y++ {
		for x := d.Min.X; x < d.Max.X; x++ {
			if c := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA); c.A == 255 && !seen[c] && len(r.pal) < 255 {
				seen[c] = true
				r.pal = append(r.pal, c)
			}
		}
	}
	for _, c := range []color.NRGBA{selectColor, markColor['?'], markColor['✓'], markColor['z'], markColor['m'], markColor['!']} {
		if !seen[c] && len(r.pal) < 255 {
			r.pal = append(r.pal, c)
		}
	}
	return r.pal, nil
}

// EncodePart turns the part of a picture inside rect into a sixel sequence in the given colours.
func EncodePart(img *image.NRGBA, rect image.Rectangle, pal color.Palette) ([]byte, error) {
	part := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(part, part.Bounds(), img, rect.Min, draw.Src)
	var buf bytes.Buffer
	e := sixel.NewEncoder(&buf)
	e.Palette = pal
	if err := e.Encode(part); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
