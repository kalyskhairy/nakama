package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"time"

	"github.com/kalyskhairy/nakama/internal/demo"
	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/theme"
)

const (
	recordFPS    = 10
	recordWarmUp = 40 * time.Second // the crew walks in and sits down before the film starts
)

var recordBackdrop = color.NRGBA{0x1e, 0x1e, 0x1e, 255} // behind see-through parts of a theme

// runRecord films a short scripted scene in a theme and saves it as an animated GIF: the crew at work,
// one waiting, a visit, one going to bed, a newcomer and a subagent.
func runRecord(args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	dir := fs.String("theme", "", "theme folder, or \"demo\" for the built-in one")
	out := fs.String("out", "office.gif", "output file")
	seconds := fs.Int("seconds", 10, "length of the film")
	width := fs.Int("width", 768, "width of the film in pixels")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--theme is required")
	}
	if *dir == "demo" {
		tmp, err := os.MkdirTemp("", "nakama-demo-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if err := demo.Write(tmp); err != nil {
			return err
		}
		*dir = tmp
	}
	th, err := theme.Load(*dir)
	if err != nil {
		return err
	}
	r, err := office.NewRenderer(th)
	if err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	err = gif.EncodeAll(f, film(th, r, *seconds*recordFPS, *width))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// cast is the character of the i-th session of the film.
func cast(th *theme.Theme, i int) string {
	if n := len(th.SessionCharacters); n > 0 {
		return th.SessionCharacters[i%n]
	}
	return th.Character("default")
}

// script is who is aboard t into the film (negative t is the warm-up).
func script(th *theme.Theme, t time.Duration, now time.Time) []scene.Actor {
	lead := func(id string, i int, st scene.Status) scene.Actor {
		return scene.Actor{ID: id, Type: "session", Title: id, Status: st, LastActive: now, Lead: true, Character: cast(th, i)}
	}
	a, b, c, d := lead("a", 0, scene.Working), lead("b", 1, scene.Working), lead("c", 2, scene.Working), lead("d", 3, scene.Working)
	if t >= time.Second {
		b.Status = scene.Waiting
	}
	if t >= 2*time.Second && t < 7*time.Second {
		a.Visit = "c"
	}
	if t >= 3*time.Second {
		d.Status = scene.Idle
	}
	actors := []scene.Actor{a, b, c, d}
	if t >= 4*time.Second {
		actors = append(actors, lead("e", 4, scene.Working))
	}
	if t >= 5*time.Second {
		actors = append(actors, scene.Actor{ID: "sub:a:1", Type: "general-purpose", Title: "sub", Status: scene.Working, LastActive: now, Character: cast(th, 0)})
	}
	return actors
}

// film draws the frames. Each frame after the first holds only the rectangle that changed.
func film(th *theme.Theme, r *office.Renderer, frames, width int) *gif.GIF {
	bg := th.Background.Bounds()
	scale := float64(width) / float64(bg.Dx())
	w := scene.NewWorld(th)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	step := time.Second / recordFPS
	for t := -recordWarmUp; t < 0; t += step {
		w.Update(script(th, t, start.Add(t)), start.Add(t))
	}
	pal, _ := r.Palette()
	pal = append(append(color.Palette(nil), pal...), recordBackdrop)
	index := map[color.Color]uint8{}
	out := &gif.GIF{}
	var prev *image.NRGBA
	for i := 0; i < frames; i++ {
		t := time.Duration(i) * step
		w.Update(script(th, t, start.Add(t)), start.Add(t))
		pic := r.Compose(w.Placed(), "", scale)
		full := image.NewNRGBA(pic.Bounds())
		draw.Draw(full, full.Bounds(), image.NewUniform(recordBackdrop), image.Point{}, draw.Src)
		draw.Draw(full, full.Bounds(), pic, image.Point{}, draw.Over)
		box := full.Bounds()
		if prev != nil {
			box = changed(prev, full)
			if box.Empty() {
				box = image.Rect(0, 0, 1, 1) // a frame must hold something; this pixel did not change
			}
		}
		pm := image.NewPaletted(box, pal)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				c := full.NRGBAAt(x, y)
				k, ok := index[c]
				if !ok {
					k = uint8(pal.Index(c))
					index[c] = k
				}
				pm.SetColorIndex(x, y, k)
			}
		}
		out.Image = append(out.Image, pm)
		out.Delay = append(out.Delay, 100/recordFPS)
		out.Disposal = append(out.Disposal, gif.DisposalNone)
		prev = full
	}
	return out
}

// changed is the smallest rectangle holding every pixel that differs between two pictures.
func changed(a, b *image.NRGBA) image.Rectangle {
	box := image.Rectangle{}
	r := b.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.NRGBAAt(x, y) != b.NRGBAAt(x, y) {
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return box
}
