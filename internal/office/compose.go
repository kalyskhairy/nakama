// Package office turns the scene into pictures: composing the frame, fitting it to the
// terminal, and encoding it as sixel.
package office

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"

	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/sprites"
	"github.com/kalyskhairy/nakama/internal/theme"
)

type scaledKey struct {
	char  string
	scale float64
}

type Renderer struct {
	th     *theme.Theme
	sheets map[string]*sprites.Sheet
	walkH  map[string]int // tallest walking frame per character
	cache  map[scaledKey]*sprites.Sheet
	bg     *image.NRGBA
	bgAt   float64
	pal    color.Palette
}

// NewRenderer loads the sprites of every character in the theme.
func NewRenderer(t *theme.Theme) (*Renderer, error) {
	r := &Renderer{th: t, sheets: map[string]*sprites.Sheet{}, walkH: map[string]int{}, cache: map[scaledKey]*sprites.Sheet{}}
	for name, dir := range t.Characters {
		s, err := sprites.Load(dir)
		if err != nil {
			return nil, fmt.Errorf("character %q: %w", name, err)
		}
		r.sheets[name] = s
		for _, ri := range []int{0, 1, 2, 3} {
			for _, f := range s.Rows[ri] {
				r.walkH[name] = max(r.walkH[name], f.Bounds().Dy())
			}
		}
	}
	return r, nil
}

func (r *Renderer) background(scale float64) *image.NRGBA {
	if r.bg != nil && r.bgAt == scale {
		return r.bg
	}
	b := r.th.Background.Bounds()
	src := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), r.th.Background, b.Min, draw.Src)
	r.bg, r.bgAt = sprites.Resize(src, int(math.Round(float64(b.Dx())*scale)), int(math.Round(float64(b.Dy())*scale))), scale
	return r.bg
}

// sheet returns the character's frames scaled so a walking frame is CharHeight (times
// its own height factor) of the picture.
func (r *Renderer) sheet(char string, scale float64) *sprites.Sheet {
	k := scaledKey{char, scale}
	if s, ok := r.cache[k]; ok {
		return s
	}
	src := r.sheets[char]
	if src == nil || r.walkH[char] == 0 {
		return nil
	}
	target := r.th.CharHeight * float64(r.th.Background.Bounds().Dy()) * scale
	if h := r.th.Heights[char]; h > 0 {
		target *= h
	}
	f := target / float64(r.walkH[char])
	out := &sprites.Sheet{}
	for ri, row := range src.Rows {
		for _, fr := range row {
			out.Rows[ri] = append(out.Rows[ri], sprites.Resize(fr, int(math.Round(float64(fr.Bounds().Dx())*f)), int(math.Round(float64(fr.Bounds().Dy())*f))))
		}
	}
	r.cache[k] = out
	return out
}

func wrap(i, n int) int { return ((i % n) + n) % n }

func pick(s *sprites.Sheet, p scene.Placed) *image.NRGBA {
	var row []*image.NRGBA
	switch p.Anim {
	case scene.Walk:
		row = s.Rows[wrap(p.Facing, 4)]
	case scene.Sleep:
		row = s.Rows[4]
	case scene.Work:
		row = s.Rows[5]
	default:
		return s.Rows[0][0]
	}
	return row[wrap(p.Frame, len(row))]
}

func dimmed(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for i := 0; i+3 < len(src.Pix); i += 4 {
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = src.Pix[i]/2, src.Pix[i+1]/2, src.Pix[i+2]/2, src.Pix[i+3]
	}
	return out
}

// shadow washes a character with dim blue, so a subagent is not taken for a chat of its own.
func shadow(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for i := 0; i+3 < len(src.Pix); i += 4 {
		r, g, b := int(src.Pix[i]), int(src.Pix[i+1]), int(src.Pix[i+2])
		out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = uint8(r*2/5+10), uint8(g*9/20+25), uint8(b*11/20+70), src.Pix[i+3]
	}
	return out
}

var glyphs = map[rune][7]string{
	'?': {".###.", "#...#", "....#", "...#.", "..#..", ".....", "..#.."},
	'✓': {".....", "....#", "...#.", "#..#.", ".##..", "..#..", "....."},
	'!': {"..#..", "..#..", "..#..", "..#..", "..#..", ".....", "..#.."},
	'm': {"#####", "#...#", "#.#.#", "#...#", "#####", ".#...", "#...."},
	'z': {"#####", "...#.", "..#..", ".#...", "#####", ".....", "....."},
}

var markColor = map[rune]color.NRGBA{'?': {255, 165, 0, 255}, '✓': {60, 200, 90, 255}, 'z': {170, 200, 255, 255}, 'm': {255, 255, 255, 255}, '!': {255, 70, 60, 255}}
var selectColor = color.NRGBA{255, 220, 0, 255}

func put(im *image.NRGBA, x, y, size int, c color.NRGBA) {
	for dy := 0; dy < size; dy++ {
		for dx := 0; dx < size; dx++ {
			if p := image.Pt(x+dx, y+dy); p.In(im.Bounds()) {
				im.SetNRGBA(p.X, p.Y, c)
			}
		}
	}
}

func drawMark(im *image.NRGBA, m rune, cx, top, unit int) {
	g, ok := glyphs[m]
	if !ok {
		return
	}
	x0, y0 := cx-5*unit/2, top-2*unit-7*unit
	for row, line := range g {
		for col, ch := range line {
			if ch == '#' {
				put(im, x0+col*unit, y0+row*unit, unit, markColor[m])
			}
		}
	}
}

func drawSelector(im *image.NRGBA, cx, top, unit int, hasMark bool) {
	off := 2 * unit
	if hasMark {
		off += 8 * unit
	}
	for row := 0; row < 4; row++ { // a triangle pointing down
		half := 3 - row
		for dx := -half; dx <= half; dx++ {
			put(im, cx+dx*unit-unit/2, top-off-(4-row)*unit, unit, selectColor)
		}
	}
}

// Bounds is where a placed actor is drawn on a picture of size w x h made at this scale.
func (r *Renderer) Bounds(p scene.Placed, w, h int, scale float64) image.Rectangle {
	s := r.sheet(p.Character, scale)
	if s == nil {
		return image.Rectangle{}
	}
	f := pick(s, p).Bounds()
	cx, feet := int(math.Round(p.P.X*float64(w))), int(math.Round(p.P.Y*float64(h)))
	return image.Rect(cx-f.Dx()/2, feet-f.Dy(), cx-f.Dx()/2+f.Dx(), feet)
}

// Compose draws the office: the background, then the actors from the top of the picture to the
// bottom so lower ones stand in front, with their marks and the selection marker.
func (r *Renderer) Compose(placed []scene.Placed, selected string, scale float64) *image.NRGBA {
	bg := r.background(scale)
	im := image.NewNRGBA(bg.Bounds())
	copy(im.Pix, bg.Pix)
	W, H := bg.Bounds().Dx(), bg.Bounds().Dy()
	unit := max(1, int(math.Round(float64(H)/320)))

	order := append([]scene.Placed(nil), placed...)
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].P.Y != order[j].P.Y {
			return order[i].P.Y < order[j].P.Y
		}
		return order[i].ID < order[j].ID
	})
	for _, p := range order {
		s := r.sheet(p.Character, scale)
		if s == nil {
			continue
		}
		f := pick(s, p)
		if p.Sub {
			f = shadow(f)
		}
		if p.Dim {
			f = dimmed(f)
		}
		cx, feet := int(math.Round(p.P.X*float64(W))), int(math.Round(p.P.Y*float64(H)))
		w, h := f.Bounds().Dx(), f.Bounds().Dy()
		top := feet - h
		draw.Draw(im, image.Rect(cx-w/2, top, cx-w/2+w, feet), f, f.Bounds().Min, draw.Over)
		if p.Mark != 0 {
			drawMark(im, p.Mark, cx, top, unit)
		}
		if p.ID == selected {
			drawSelector(im, cx, top, unit, p.Mark != 0)
		}
	}
	return im
}
