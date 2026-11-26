package office

import "testing"

func TestHeightsScaleOneCharacter(t *testing.T) {
	r := testRenderer(t)
	r.th.Heights = map[string]float64{"d": 1.5}
	c, d := r.sheet("c", 1).Rows[0][0].Bounds().Dy(), r.sheet("d", 1).Rows[0][0].Bounds().Dy()
	if c != 20 || d != 30 {
		t.Fatalf("c %d d %d, want 20 and 30", c, d)
	}
}
