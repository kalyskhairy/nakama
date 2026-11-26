// Package input decodes raw terminal bytes into events and documents the keys.
package input

// Mode says where keys go.
type Mode int

const (
	Work    Mode = iota // the three panels, focus in the lists
	Chat                // focus in the chat panel: every key belongs to Claude except the focus key and F1
	Home                // the office
	Overlay             // help, folder filter, new session or details on top of everything
)
