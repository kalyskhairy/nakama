package scene

import (
	"math"

	"github.com/kalyskhairy/nakama/internal/theme"
)

// Path is the shortest route over the walking graph, as node ids from..to inclusive.
// It returns nil when either node is unknown or there is no route.
func Path(nodes map[string]theme.Point, edges [][2]string, from, to string) []string {
	if _, ok := nodes[from]; !ok {
		return nil
	}
	if _, ok := nodes[to]; !ok {
		return nil
	}
	adj := map[string][]string{}
	for _, e := range edges {
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}
	dist := map[string]float64{from: 0}
	prev := map[string]string{}
	done := map[string]bool{}
	for {
		cur, best := "", math.Inf(1)
		for n, d := range dist { // graphs are tiny: a linear scan is enough
			if !done[n] && d < best {
				cur, best = n, d
			}
		}
		if cur == "" {
			return nil
		}
		if cur == to {
			break
		}
		done[cur] = true
		for _, m := range adj[cur] {
			a, b := nodes[cur], nodes[m]
			nd := best + math.Hypot(a.X-b.X, a.Y-b.Y)
			if d, seen := dist[m]; !seen || nd < d {
				dist[m], prev[m] = nd, cur
			}
		}
	}
	route := []string{to}
	for n := to; n != from; {
		n = prev[n]
		route = append([]string{n}, route...)
	}
	return route
}
