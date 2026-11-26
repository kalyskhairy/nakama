// Package demo draws a small office theme of its own, so the office works before the owner has
// made a theme. All artwork is drawn here, by code.
package demo

import (
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/kalyskhairy/nakama/internal/sprites"
)

const (
	zoom   = 4 // one art pixel is zoom x zoom picture pixels
	width  = 384
	height = 184
)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

var (
	ink   = rgb(0x2a1c14)
	skin  = rgb(0xf2c39b)
	white = rgb(0xf4f1e6)
)

type canvas struct{ *image.NRGBA }

func newCanvas(w, h int) canvas { return canvas{image.NewNRGBA(image.Rect(0, 0, w, h))} }

func (c canvas) box(x, y, w, h int, col color.NRGBA) {
	draw.Draw(c, image.Rect(x, y, x+w, y+h), image.NewUniform(col), image.Point{}, draw.Src)
}

// framed is a box with a dark edge.
func (c canvas) framed(x, y, w, h int, col color.NRGBA) {
	c.box(x, y, w, h, ink)
	c.box(x+1, y+1, w-2, h-2, col)
}

func (c canvas) enlarge() *image.NRGBA {
	b := c.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*zoom, b.Dy()*zoom))
	for y := 0; y < b.Dy()*zoom; y++ {
		for x := 0; x < b.Dx()*zoom; x++ {
			out.SetNRGBA(x, y, c.NRGBAAt(x/zoom, y/zoom))
		}
	}
	return out
}

// outline puts a dark pixel on every empty pixel that touches a drawn one.
func (c canvas) outline() {
	b := c.Bounds()
	var edge []image.Point
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if c.NRGBAAt(x, y).A != 0 {
				continue
			}
			for _, d := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if p := image.Pt(x+d.X, y+d.Y); p.In(b) && c.NRGBAAt(p.X, p.Y).A != 0 {
					edge = append(edge, image.Pt(x, y))
					break
				}
			}
		}
	}
	for _, p := range edge {
		c.SetNRGBA(p.X, p.Y, ink)
	}
}

// ---- characters ----

type person struct {
	name               string
	hair, shirt, pants color.NRGBA
	hat                color.NRGBA // zero = bare head
}

var people = []person{
	{name: "captain", hair: rgb(0x4a2f1d), shirt: rgb(0x2f5fb3), pants: rgb(0x27324a), hat: rgb(0xf4f1e6)},
	{name: "scholar", hair: rgb(0x1f1b2e), shirt: rgb(0x7a4fa3), pants: rgb(0x3a2f4d)},
	{name: "cook", hair: rgb(0xd9a441), shirt: rgb(0xf4f1e6), pants: rgb(0x3b3b3b), hat: rgb(0xffffff)},
	{name: "smith", hair: rgb(0xb2461f), shirt: rgb(0x6d7a3a), pants: rgb(0x4a3a28)},
	{name: "gardener", hair: rgb(0x2c2018), shirt: rgb(0x3f9b6a), pants: rgb(0x5b4630), hat: rgb(0xd9b25f)},
	{name: "painter", hair: rgb(0xc65a86), shirt: rgb(0xe0713c), pants: rgb(0x35506b)},
}

const (
	down = iota
	left
	right
	up
)

// standing draws a 16x24 figure. step 0..3 is the walk cycle; arm lifts the working arm.
func (p person) standing(dir, step, arm int) canvas {
	c := newCanvas(16, 24)
	bob := step % 2
	lift := [2]int{}
	switch step {
	case 1:
		lift[0] = 2
	case 3:
		lift[1] = 2
	}
	top := 3 - bob
	c.box(5, 18, 2, 5-lift[0], p.pants) // legs
	c.box(9, 18, 2, 5-lift[1], p.pants)
	c.box(4, top+9, 8, 7+bob, p.shirt) // body
	c.box(3, top+10, 1, 4, p.shirt)    // arms
	c.box(12, top+10-arm, 1, 4, p.shirt)
	c.box(3, top+14, 1, 1, skin)
	c.box(12, top+14-arm, 1, 1, skin)
	c.box(4, top+1, 8, 8, skin) // head
	c.box(4, top, 8, 3, p.hair)
	switch dir {
	case down:
		c.box(4, top+3, 1, 3, p.hair)
		c.box(11, top+3, 1, 3, p.hair)
		c.box(6, top+5, 1, 2, ink)
		c.box(9, top+5, 1, 2, ink)
	case up:
		c.box(4, top+1, 8, 7, p.hair)
	case right:
		c.box(4, top+3, 3, 4, p.hair)
		c.box(10, top+5, 1, 2, ink)
	case left:
		c.box(9, top+3, 3, 4, p.hair)
		c.box(5, top+5, 1, 2, ink)
	}
	if p.hat.A != 0 {
		c.box(4, top-2, 8, 2, p.hat)
		c.box(3, top, 10, 1, p.hat)
	}
	c.outline()
	return c
}

// sleeping draws the figure lying down; breath raises the blanket.
func (p person) sleeping(breath int) canvas {
	c := newCanvas(24, 12)
	c.box(1, 3, 8, 8, p.hair)
	c.box(3, 4, 6, 6, skin)
	c.box(5, 7, 2, 1, ink) // closed eyes
	c.box(9, 4-breath, 13, 7+breath, p.shirt)
	c.box(18, 5, 5, 5, p.pants)
	c.outline()
	return c
}

// working is standing at something, one arm going up and down, a tool in the hand.
func (p person) working(step int) canvas {
	arm := []int{0, 2, 4, 2}[step]
	c := p.standing(down, 0, arm)
	c.box(13, 15-arm, 2, 3, rgb(0xc9ced6))
	return c
}

func (p person) sheet() *sprites.Sheet {
	s := &sprites.Sheet{}
	for row, dir := range []int{down, left, right, up} {
		for step := 0; step < 4; step++ {
			s.Rows[row] = append(s.Rows[row], p.standing(dir, step, 0).enlarge())
		}
	}
	s.Rows[4] = []*image.NRGBA{p.sleeping(0).enlarge(), p.sleeping(1).enlarge()}
	for step := 0; step < 4; step++ {
		s.Rows[5] = append(s.Rows[5], p.working(step).enlarge())
	}
	return s
}

// ---- the floor plan ----

func background() *image.NRGBA {
	c := newCanvas(width, height)
	c.box(0, 0, width, height, rgb(0x2b6ca3)) // water
	for y := 6; y < height; y += 14 {
		for x := (y * 7) % 23; x < width; x += 29 {
			c.box(x, y, 6, 1, rgb(0x5b97c4))
		}
	}
	c.framed(10, 6, width-20, height-12, rgb(0x6b4527)) // walls
	floor := func(x, y, w, h int, a, b color.NRGBA) {
		c.box(x, y, w, h, a)
		for yy := y + 5; yy < y+h; yy += 6 {
			c.box(x, yy, w, 1, b)
		}
	}
	floor(14, 76, width-28, 32, rgb(0xc99b66), rgb(0xb9894f)) // corridor
	rooms := []struct{ x, y int }{{14, 10}, {134, 10}, {254, 10}, {14, 112}, {134, 112}, {254, 112}}
	tints := []uint32{0xa9744a, 0x9d7b55, 0xd8c9a5, 0x8d8d92, 0x7d6a8e, 0x6fa354}
	for i, r := range rooms {
		floor(r.x, r.y, 116, 62, rgb(tints[i]), rgb(tints[i]-0x0e0e0e))
		door := r.y + 62
		if r.y > 100 {
			door = r.y - 4
		}
		c.box(r.x+46, door, 24, 4, rgb(0xc99b66))
	}
	// library: shelves and a desk
	for i, x := range []int{20, 52, 84} {
		c.framed(x, 12, 28, 14, rgb(0x70451f))
		for b := 0; b < 11; b++ {
			c.box(x+3+b*2, 15, 2, 8, rgb([]uint32{0xc0392b, 0x2f7fb3, 0xd9a441, 0x3f9b6a}[(b+i)%4]))
		}
	}
	desk := func(x, y int) {
		c.framed(x, y, 30, 12, rgb(0x8a5a30))
		c.framed(x+10, y+2, 10, 6, rgb(0x9fd4e8))
	}
	desk(50, 34)
	// office: three desks
	desk(142, 26)
	desk(208, 26)
	desk(175, 14)
	// kitchen: stove, counter and table
	c.framed(262, 12, 100, 14, rgb(0xb9c0c7))
	for x := 270; x < 300; x += 9 {
		c.framed(x, 15, 7, 7, rgb(0x3b3b3b))
	}
	c.framed(290, 36, 44, 14, rgb(0x8a5a30))
	c.framed(306, 40, 10, 6, rgb(0xe0713c))
	// workshop: bench and tools
	c.framed(30, 150, 60, 14, rgb(0x6b4527))
	for x := 36; x < 84; x += 10 {
		c.box(x, 153, 6, 2, rgb(0xc9ced6))
		c.box(x+2, 155, 2, 6, rgb(0x70451f))
	}
	// bedroom: six beds
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			x, y := 142+col*36, 118+row*28
			c.framed(x, y, 28, 22, rgb(0x4d74c0))
			c.box(x+2, y+2, 8, 18, white)
		}
	}
	// garden: bushes, a bench and an easel
	for _, p := range []image.Point{{258, 116}, {352, 116}, {258, 158}, {352, 158}, {304, 116}} {
		c.framed(p.X, p.Y, 14, 12, rgb(0x2f7a3a))
		c.box(p.X+3, p.Y+3, 2, 2, rgb(0xf2d14e))
		c.box(p.X+8, p.Y+6, 2, 2, rgb(0xd6577a))
	}
	desk(268, 140)
	c.framed(326, 132, 18, 20, white)
	c.box(330, 137, 6, 6, rgb(0xe0713c))
	return c.enlarge()
}

// ---- the theme file ----

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func at(x, y int) point {
	r := func(v float64) float64 { return math.Round(v*10000) / 10000 }
	return point{r(float64(x) / width), r(float64(y) / height)}
}

func themeJSON() ([]byte, error) {
	nodes := map[string]point{"entrance": at(18, 92), "a": at(72, 92), "b": at(192, 92), "c": at(312, 92), "end": at(364, 92)}
	edges := [][2]string{{"entrance", "a"}, {"a", "b"}, {"b", "c"}, {"c", "end"}}
	for name, x := range map[string]int{"A": 72, "B": 192, "C": 312} {
		hub := map[string]string{"A": "a", "B": "b", "C": "c"}[name]
		nodes["up"+name], nodes["down"+name] = at(x, 76), at(x, 108)
		edges = append(edges, [2]string{hub, "up" + name}, [2]string{hub, "down" + name})
	}
	var stations []map[string]any
	add := func(id, kind, room, node string, x, y int, owner ...string) {
		p := at(x, y)
		s := map[string]any{"id": id, "x": p.X, "y": p.Y, "kind": kind, "room": room, "node": node}
		if len(owner) > 0 {
			s["for"] = owner
		}
		stations = append(stations, s)
	}
	add("library", "work", "library", "upA", 65, 62, "scholar", "Explore")
	add("kitchen", "work", "kitchen", "upC", 312, 66, "cook")
	add("workshop", "work", "workshop", "downA", 60, 148, "smith", "general-purpose")
	add("easel", "work", "garden", "downC", 335, 168, "painter")
	add("garden", "work", "garden", "downC", 283, 164, "gardener")
	add("desk-left", "work", "office", "upB", 157, 52)
	add("desk-right", "work", "office", "upB", 223, 52)
	add("desk-back", "work", "office", "upB", 190, 40, "captain", "Plan")
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			add("bed-"+string(rune('1'+row*3+col)), "rest", "bedroom", "downB", 156+col*36, 136+row*28)
		}
	}
	chars, cast := map[string]string{}, []string{}
	for _, p := range people {
		chars[p.name], cast = "sprites/"+p.name, append(cast, p.name)
	}
	return json.MarshalIndent(map[string]any{
		"name": "demo", "background": map[string]any{"file": "background.png"}, "charHeight": 0.14,
		"characters": chars, "agentTypes": map[string]string{"default": "captain"}, "sessionCharacters": cast,
		"corridor": []string{"entrance", "end"}, "nodes": nodes, "edges": edges, "entrance": "entrance",
		"stations": stations,
	}, "", "  ")
}

// Write puts the demo theme in dir. A theme that is already there is left alone.
func Write(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "theme.json")); err == nil {
		return nil
	}
	for _, p := range people {
		if err := p.sheet().Save(filepath.Join(dir, "sprites", p.name)); err != nil {
			return err
		}
	}
	f, err := os.Create(filepath.Join(dir, "background.png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, background())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	b, err := themeJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "theme.json"), b, 0o644) // last: it marks the theme as complete
}
