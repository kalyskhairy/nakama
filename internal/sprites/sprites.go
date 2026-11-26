// Package sprites cuts a ChatGPT-style character sheet (6 rows on a magenta background) into frames.
package sprites

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// Sheet holds the frames of one character. Row indexes: 0 down, 1 left, 2 right, 3 up, 4 sleep, 5 work.
type Sheet struct{ Rows [6][]*image.NRGBA }

var rowNames = [6]string{"down", "left", "right", "up", "sleep", "work"}

// isBG reports whether c is (close to) the magenta key colour.
func isBG(c color.NRGBA) bool { return int(min(c.R, c.B))-int(c.G) > 110 }

type span struct{ a, b int }

func runs(has []bool, gap int) []span {
	var out []span
	start, last := -1, -1
	for i, h := range has {
		if !h {
			continue
		}
		if start < 0 {
			start = i
		} else if i-last > gap {
			out = append(out, span{start, last})
			start = i
		}
		last = i
	}
	if start >= 0 {
		out = append(out, span{start, last})
	}
	return out
}

func sub(im *image.NRGBA, r image.Rectangle) *image.NRGBA {
	o := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(o, o.Bounds(), im, r.Min, draw.Src)
	return o
}

// scale resizes with an area average over premultiplied colour.
func scale(im *image.NRGBA, w, h int) *image.NRGBA {
	w, h = max(w, 1), max(h, 1)
	sw, sh := im.Bounds().Dx(), im.Bounds().Dy()
	o := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
			var r, g, b, a, n int
			for yy := y0; yy < y1 && yy < sh; yy++ {
				for xx := x0; xx < x1 && xx < sw; xx++ {
					c := im.NRGBAAt(xx, yy)
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					b += int(c.B) * int(c.A)
					a += int(c.A)
					n++
				}
			}
			if a > 0 {
				o.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / n)})
			}
		}
	}
	return o
}

// Resize scales an image to w x h with an area average.
func Resize(im *image.NRGBA, w, h int) *image.NRGBA { return scale(im, w, h) }

func mirror(im *image.NRGBA) *image.NRGBA {
	b := im.Bounds()
	o := image.NewNRGBA(b)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			o.SetNRGBA(b.Dx()-1-x, y, im.NRGBAAt(x, y))
		}
	}
	return o
}

// Slice cuts the sheet into frames. Walking frames are scaled so the tallest is targetH pixels;
// every other frame uses the same scale. The left row is always the mirror of the right row
// (generated sheets draw both facing right).
func Slice(src image.Image, targetH int) (*Sheet, error) {
	b := src.Bounds()
	im := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(im, im.Bounds(), src, b.Min, draw.Src)
	w, h := im.Bounds().Dx(), im.Bounds().Dy()

	has := make([][]bool, h)
	for y := range has {
		has[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			if c := im.NRGBAAt(x, y); isBG(c) || c.A == 0 {
				im.SetNRGBA(x, y, color.NRGBA{})
			} else {
				has[y][x] = true
			}
		}
	}
	rowHas := make([]bool, h)
	for y := 1; y < h-1; y++ { // drop pixels with fewer than two opaque neighbours (noise)
		for x := 1; x < w-1; x++ {
			if !has[y][x] {
				continue
			}
			n := 0
			for _, d := range [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
				if has[y+d[1]][x+d[0]] {
					n++
				}
			}
			if n >= 2 {
				rowHas[y] = true
			} else {
				im.SetNRGBA(x, y, color.NRGBA{})
				has[y][x] = false
			}
		}
	}
	bands := runs(rowHas, 12)
	if len(bands) != 6 {
		return nil, fmt.Errorf("lembar sprite harus punya 6 baris, ditemukan %d", len(bands))
	}
	var rows [6][]*image.NRGBA
	for i, bd := range bands {
		col := make([]bool, w)
		for y := bd.a; y <= bd.b; y++ {
			for x := 0; x < w; x++ {
				if has[y][x] {
					col[x] = true
				}
			}
		}
		for _, c := range runs(col, 20) {
			rows[i] = append(rows[i], sub(im, image.Rect(c.a, bd.a, c.b+1, bd.b+1)))
		}
		if len(rows[i]) == 0 {
			return nil, errors.New("baris sprite kosong")
		}
	}

	maxH := 0
	for _, ri := range []int{0, 1, 2, 3} {
		for _, f := range rows[ri] {
			maxH = max(maxH, f.Bounds().Dy())
		}
	}
	if maxH == 0 || targetH <= 0 {
		return nil, errors.New("tinggi sprite tidak valid")
	}
	s := &Sheet{}
	for ri := range rows {
		for _, f := range rows[ri] {
			fw, fh := f.Bounds().Dx(), f.Bounds().Dy()
			s.Rows[ri] = append(s.Rows[ri], scale(f, (fw*targetH+maxH/2)/maxH, (fh*targetH+maxH/2)/maxH))
		}
	}
	s.Rows[1] = nil
	for _, f := range s.Rows[2] {
		s.Rows[1] = append(s.Rows[1], mirror(f))
	}
	return s, nil
}

func (s *Sheet) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for ri, row := range s.Rows {
		for i, f := range row {
			fh, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s_%d.png", rowNames[ri], i)))
			if err != nil {
				return err
			}
			err = png.Encode(fh, f)
			if cerr := fh.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func Load(dir string) (*Sheet, error) {
	s := &Sheet{}
	for ri, name := range rowNames {
		for i := 0; ; i++ {
			fh, err := os.Open(filepath.Join(dir, fmt.Sprintf("%s_%d.png", name, i)))
			if err != nil {
				break
			}
			im, err := png.Decode(fh)
			fh.Close()
			if err != nil {
				return nil, fmt.Errorf("sprite %s_%d.png rusak: %w", name, i, err)
			}
			n := image.NewNRGBA(im.Bounds())
			draw.Draw(n, n.Bounds(), im, im.Bounds().Min, draw.Src)
			s.Rows[ri] = append(s.Rows[ri], n)
		}
		if len(s.Rows[ri]) == 0 {
			return nil, fmt.Errorf("sprite %s tidak ditemukan di %s", name, dir)
		}
	}
	return s, nil
}
