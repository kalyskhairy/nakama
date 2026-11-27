package history

import "strings"

// price is a model's API list price in dollars per million tokens. Cache writes cost 1.25x the input
// price for a 5-minute entry and 2x for a 1-hour one.
type price struct{ in, out, read float64 }

// prices by model id prefix, longest first where one id starts with another.
// shortcut: list prices as of 2026-11; update the table when prices change or a model is added.
var prices = []struct {
	prefix string
	p      price
}{
	{"claude-fable-5-2", price{8, 40, 0.20}},
	{"claude-mythos-5-2", price{8, 40, 0.20}},
	{"claude-fable-5-1", price{10, 50, 0.25}},
	{"claude-mythos-5-1", price{10, 50, 0.25}},
	{"claude-fable-5", price{10, 50, 1}},
	{"claude-mythos-5", price{10, 50, 1}},
	{"claude-opus-5-5", price{4, 20, 0.20}},
	{"claude-opus-5", price{5, 25, 0.50}},
	{"claude-opus-4-8", price{5, 25, 0.50}},
	{"claude-opus-4-7", price{5, 25, 0.50}},
	{"claude-opus-4-6", price{5, 25, 0.50}},
	{"claude-sonnet-5-5", price{2, 10, 0.10}},
	{"claude-sonnet-5", price{2, 10, 0.20}},
	{"claude-sonnet-4-6", price{3, 15, 0.30}},
	{"claude-haiku-5-5", price{0.10, 0.50, 0.01}}, // shortcut: prompts over 100K tokens cost more; not modelled
	{"claude-haiku-4-5", price{1, 5, 0.10}},
}

func priceOf(model string) (price, bool) {
	for _, e := range prices {
		if strings.HasPrefix(model, e.prefix) {
			return e.p, true
		}
	}
	return price{}, false
}

// cost is what one message's tokens cost at list price; ok is false for a model without a price.
func cost(model string, input, write5m, write1h, read, output int) (float64, bool) {
	p, ok := priceOf(model)
	if !ok {
		return 0, false
	}
	m := 1e-6
	return (float64(input)*p.in + float64(write5m)*p.in*1.25 + float64(write1h)*p.in*2 +
		float64(read)*p.read + float64(output)*p.out) * m, true
}
