package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/kalyskhairy/nakama/internal/office"
	"github.com/kalyskhairy/nakama/internal/scene"
	"github.com/kalyskhairy/nakama/internal/sprites"
	"github.com/kalyskhairy/nakama/internal/theme"
)

// runTheme handles `nakama theme <subcommand>`.
func runTheme(args []string) error {
	if len(args) > 0 && args[0] == "preview" {
		return runPreview(args[1:])
	}
	if len(args) == 0 || args[0] != "import-sheet" {
		return fmt.Errorf("pemakaian: nakama theme import-sheet --sheet <png> --theme <dir> --name <karakter> [--height 64]\n        nakama theme preview --theme <dir> [--out preview.png]")
	}
	fs := flag.NewFlagSet("import-sheet", flag.ContinueOnError)
	sheet := fs.String("sheet", "", "lembar sprite (PNG, latar magenta, 6 baris)")
	theme := fs.String("theme", "", "direktori tema tujuan")
	name := fs.String("name", "", "nama karakter")
	height := fs.Int("height", 64, "tinggi frame jalan (piksel)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *sheet == "" || *theme == "" || *name == "" {
		return fmt.Errorf("--sheet, --theme, dan --name wajib diisi")
	}
	f, err := os.Open(*sheet)
	if err != nil {
		return err
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("%s bukan PNG yang valid: %w", *sheet, err)
	}
	s, err := sprites.Slice(im, *height)
	if err != nil {
		return err
	}
	out := filepath.Join(*theme, "sprites", *name)
	if err := s.Save(out); err != nil {
		return err
	}
	fmt.Printf("%d frame disimpan di %s\n", len(s.Rows[0])+len(s.Rows[1])+len(s.Rows[2])+len(s.Rows[3])+len(s.Rows[4])+len(s.Rows[5]), out)
	return nil
}

func line(im *image.NRGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	n := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		x, y := int(x0+(x1-x0)*f), int(y0+(y1-y0)*f)
		if image.Pt(x, y).In(im.Bounds()) {
			im.SetNRGBA(x, y, c)
		}
	}
}

func cross(im *image.NRGBA, x, y, arm int, c color.NRGBA) {
	line(im, float64(x-arm), float64(y), float64(x+arm), float64(y), c)
	line(im, float64(x), float64(y-arm), float64(x), float64(y+arm), c)
}

// runPreview draws a theme as it will be used: a character standing on every station, the walking
// graph (cyan lines, white nodes) and the stations (green = work, blue = rest) on top.
func runPreview(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	dir := fs.String("theme", "", "direktori tema")
	out := fs.String("out", "preview.png", "berkas keluaran")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return fmt.Errorf("--theme wajib diisi")
	}
	th, err := theme.Load(*dir)
	if err != nil {
		return err
	}
	r, err := office.NewRenderer(th)
	if err != nil {
		return err
	}
	var placed []scene.Placed
	for i, s := range th.Stations {
		char := th.Character("default")
		if n := len(th.SessionCharacters); n > 0 {
			char = th.SessionCharacters[i%n] // show the whole cast side by side
			for _, f := range s.For {
				if _, ok := th.Characters[f]; ok {
					char = f // its owner
				}
			}
		}
		placed = append(placed, scene.Placed{ID: s.ID, Character: char, P: s.Point})
	}
	im := r.Compose(placed, "", 1)
	W, H := float64(im.Bounds().Dx()), float64(im.Bounds().Dy())
	for _, e := range th.Edges {
		a, b := th.Nodes[e[0]], th.Nodes[e[1]]
		line(im, a.X*W, a.Y*H, b.X*W, b.Y*H, color.NRGBA{0, 255, 255, 255})
	}
	for _, n := range th.Nodes {
		cross(im, int(n.X*W), int(n.Y*H), 3, color.NRGBA{255, 255, 255, 255})
	}
	for _, s := range th.Stations {
		c := color.NRGBA{0, 255, 0, 255}
		if s.Kind == "rest" {
			c = color.NRGBA{80, 140, 255, 255}
		}
		x, y := int(s.X*W), int(s.Y*H)
		cross(im, x, y, 6, c)
		if n, ok := th.Nodes[s.Node]; ok {
			line(im, float64(x), float64(y), n.X*W, n.Y*H, c) // the last leg the character walks
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	err = png.Encode(f, im)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
