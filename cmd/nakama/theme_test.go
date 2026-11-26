package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRunThemeRejectsBadUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"import-sheet"}, {"import-sheet", "--sheet", "x.png"}} {
		if err := runTheme(args); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
	if err := runTheme([]string{"import-sheet", "--sheet", "/nonexistent.png", "--theme", t.TempDir(), "--name", "a"}); err == nil {
		t.Error("missing sheet should fail")
	}
}

func TestRunThemeImportsASheet(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 1024, 1536))
	for i := range im.Pix {
		im.Pix[i] = 0
	}
	mag := color.NRGBA{255, 0, 255, 255}
	for y := 0; y < 1536; y++ {
		for x := 0; x < 1024; x++ {
			im.SetNRGBA(x, y, mag)
		}
	}
	for _, r := range []struct{ y0, h, n int }{{20, 200, 4}, {260, 200, 4}, {500, 200, 4}, {740, 200, 4}, {1020, 100, 2}, {1210, 200, 4}} {
		for i := 0; i < r.n; i++ {
			for y := r.y0; y < r.y0+r.h; y++ {
				for x := 60 + i*256; x < 160+i*256; x++ {
					im.SetNRGBA(x, y, color.NRGBA{200, 40, 40, 255})
				}
			}
		}
	}
	dir := t.TempDir()
	sheet := filepath.Join(dir, "sheet.png")
	f, _ := os.Create(sheet)
	png.Encode(f, im)
	f.Close()
	if err := runTheme([]string{"import-sheet", "--sheet", sheet, "--theme", dir, "--name", "hero"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sprites", "hero", "work_3.png")); err != nil {
		t.Fatal(err)
	}
}

func TestRunThemePreviewDrawsStationsAndGraph(t *testing.T) {
	themes := t.TempDir()
	writeTheme(t, themes, "demo")
	out := filepath.Join(t.TempDir(), "preview.png")
	if err := runTheme([]string{"preview", "--theme", filepath.Join(themes, "demo"), "--out", out}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := im.Bounds(); b.Dx() != 100 || b.Dy() != 50 {
		t.Fatalf("preview is %v, want the 100x50 background", b)
	}
	// the work station is at (0.5, 0.5): its marker pixel must differ from the plain background
	if c := color.NRGBAModel.Convert(im.At(50, 24)).(color.NRGBA); c == (color.NRGBA{120, 120, 120, 255}) {
		t.Fatal("nothing is drawn at the station")
	}
	if err := runTheme([]string{"preview", "--theme", filepath.Join(themes, "nope"), "--out", out}); err == nil {
		t.Fatal("a missing theme must fail")
	}
}
