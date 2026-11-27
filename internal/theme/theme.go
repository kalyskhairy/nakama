// Package theme loads a theme pack: the office background, sprite folders, stations and walking graph.
package theme

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

type Point struct{ X, Y float64 } // fractions of the background, 0..1

type Station struct {
	ID string
	Point
	Kind string // "work" | "rest"
	Room string
	For  []string // agent types it suits; empty = anyone
	Node string   // graph node it hangs off
}

type Theme struct {
	Name       string
	Dir        string
	Background image.Image
	CharHeight float64           // character height, fraction of the background height
	Characters map[string]string // character name -> absolute sprite directory
	AgentTypes map[string]string // agent type -> character name; "default" is required
	Stations   []Station
	// SessionCharacters, when set, gives every session one of these (its subagents too) instead of
	// choosing by agent type.
	SessionCharacters []string
	Heights           map[string]float64 // character name -> height relative to CharHeight; absent = 1
	Corridor          []string           // nodes of the passage where the characters not in use hang about
	CorridorLanes     int                // parallel lanes they walk in, side by side (1 = down the middle)
	Nodes             map[string]Point
	Edges             [][2]string
	Entrance          string
}

type file struct {
	Name       string `json:"name"`
	Background struct {
		File string `json:"file"`
		Crop []int  `json:"crop"`
	} `json:"background"`
	CharHeight        float64                           `json:"charHeight"`
	Characters        map[string]string                 `json:"characters"`
	AgentTypes        map[string]string                 `json:"agentTypes"`
	Nodes             map[string]struct{ X, Y float64 } `json:"nodes"`
	Edges             [][2]string                       `json:"edges"`
	Entrance          string                            `json:"entrance"`
	SessionCharacters []string                          `json:"sessionCharacters"`
	Heights           map[string]float64                `json:"heights"`
	Corridor          []string                          `json:"corridor"`
	CorridorLanes     int                               `json:"corridorLanes"`
	Stations          []struct {
		ID, Kind, Room, Node string
		X, Y                 float64
		For                  []string
	} `json:"stations"`
}

const defaultCharHeight = 0.088

func inUnit(v float64) bool { return v >= 0 && v <= 1 }

func Load(dir string) (*Theme, error) {
	b, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return nil, fmt.Errorf("cannot read theme.json: %w", err)
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("broken theme.json (json): %w", err)
	}
	t := &Theme{Name: f.Name, Dir: dir, CharHeight: f.CharHeight, AgentTypes: f.AgentTypes,
		Characters: map[string]string{}, Nodes: map[string]Point{}, Entrance: f.Entrance, Edges: f.Edges}
	if t.CharHeight <= 0 {
		t.CharHeight = defaultCharHeight
	}

	bg, err := loadBackground(dir, f.Background.File, f.Background.Crop)
	if err != nil {
		return nil, err
	}
	t.Background = bg

	for name, rel := range f.Characters {
		abs := filepath.Join(dir, rel)
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("missing sprite folder (sprite): character %q at %s", name, abs)
		}
		t.Characters[name] = abs
	}
	if _, ok := f.AgentTypes["default"]; !ok {
		return nil, fmt.Errorf("agentTypes needs a default key")
	}
	for typ, ch := range f.AgentTypes {
		if _, ok := t.Characters[ch]; !ok {
			return nil, fmt.Errorf("character %q (for type %q) is not in characters", ch, typ)
		}
	}
	for _, ch := range f.SessionCharacters {
		if _, ok := t.Characters[ch]; !ok {
			return nil, fmt.Errorf("character %q in sessionCharacters is not in characters", ch)
		}
	}
	for ch, h := range f.Heights {
		if _, ok := t.Characters[ch]; !ok || h <= 0 {
			return nil, fmt.Errorf("heights: %q must be a known character with a positive value", ch)
		}
	}
	t.SessionCharacters, t.Heights = f.SessionCharacters, f.Heights
	for id, n := range f.Nodes {
		if !inUnit(n.X) || !inUnit(n.Y) {
			return nil, fmt.Errorf("node %q is outside 0..1", id)
		}
		t.Nodes[id] = Point{n.X, n.Y}
	}
	if _, ok := t.Nodes[f.Entrance]; !ok {
		return nil, fmt.Errorf("entrance %q is not a node (node)", f.Entrance)
	}
	for _, id := range f.Corridor {
		if _, ok := t.Nodes[id]; !ok {
			return nil, fmt.Errorf("corridor names a missing node (node): %q", id)
		}
	}
	if len(f.Corridor) == 1 {
		return nil, fmt.Errorf("corridor needs at least two nodes")
	}
	t.Corridor, t.CorridorLanes = f.Corridor, f.CorridorLanes
	switch {
	case f.CorridorLanes == 0:
		t.CorridorLanes = 1
	case f.CorridorLanes < 1 || f.CorridorLanes > 5:
		return nil, fmt.Errorf("corridorLanes must be 1 to 5")
	}
	adj := map[string][]string{}
	for _, e := range f.Edges {
		if _, ok := t.Nodes[e[0]]; !ok {
			return nil, fmt.Errorf("edge names a missing node (node): %q", e[0])
		}
		if _, ok := t.Nodes[e[1]]; !ok {
			return nil, fmt.Errorf("edge names a missing node (node): %q", e[1])
		}
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}
	work, rest := 0, 0
	for _, s := range f.Stations {
		if !inUnit(s.X) || !inUnit(s.Y) {
			return nil, fmt.Errorf("station %q is outside 0..1", s.ID)
		}
		if _, ok := t.Nodes[s.Node]; !ok {
			return nil, fmt.Errorf("station %q names a missing node (node): %q", s.ID, s.Node)
		}
		switch s.Kind {
		case "work":
			work++
		case "rest":
			rest++
		default:
			return nil, fmt.Errorf("station %q: kind must be work or rest", s.ID)
		}
		t.Stations = append(t.Stations, Station{ID: s.ID, Point: Point{s.X, s.Y}, Kind: s.Kind, Room: s.Room, For: s.For, Node: s.Node})
	}
	if work == 0 || rest == 0 {
		return nil, fmt.Errorf("needs at least one work and one rest station (station)")
	}
	seen := map[string]bool{f.Entrance: true}
	queue := []string{f.Entrance}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range adj[n] {
			if !seen[m] {
				seen[m] = true
				queue = append(queue, m)
			}
		}
	}
	for id := range t.Nodes {
		if !seen[id] {
			return nil, fmt.Errorf("the graph is not connected: node %q cannot be reached from the entrance", id)
		}
	}
	return t, nil
}

func loadBackground(dir, name string, crop []int) (image.Image, error) {
	if name == "" {
		return nil, fmt.Errorf("no background file set (background)")
	}
	fh, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return nil, fmt.Errorf("background file not found (background): %w", err)
	}
	defer fh.Close()
	im, err := png.Decode(fh)
	if err != nil {
		return nil, fmt.Errorf("background is not a valid PNG (background): %w", err)
	}
	b := im.Bounds()
	r := b
	if len(crop) != 0 {
		if len(crop) != 4 || crop[2] <= 0 || crop[3] <= 0 {
			return nil, fmt.Errorf("crop must be positive [x,y,width,height] (crop)")
		}
		r = image.Rect(b.Min.X+crop[0], b.Min.Y+crop[1], b.Min.X+crop[0]+crop[2], b.Min.Y+crop[1]+crop[3])
		if !r.In(b) {
			return nil, fmt.Errorf("crop is outside the %dx%d picture (crop)", b.Dx(), b.Dy())
		}
	}
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), im, r.Min, draw.Src)
	return out, nil
}

// Character returns the character name for an agent type, falling back to "default".
func (t *Theme) Character(agentType string) string {
	if c, ok := t.AgentTypes[agentType]; ok {
		return c
	}
	return t.AgentTypes["default"]
}
