package main

import (
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordFilmsTheDemoOffice(t *testing.T) {
	out := filepath.Join(t.TempDir(), "office.gif")
	if err := runRecord([]string{"--theme", "demo", "--out", out, "--seconds", "3", "--width", "384"}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, err := gif.DecodeAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 30 || g.Image[0].Bounds().Dx() != 384 {
		t.Fatalf("%d frames, first %v", len(g.Image), g.Image[0].Bounds())
	}
	moved := false
	for _, im := range g.Image[1:] {
		moved = moved || im.Bounds().Dx() > 1
	}
	if !moved {
		t.Fatal("nothing moves")
	}
}
