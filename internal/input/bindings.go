package input

// Binding documents one key or mouse action. The help screen is built from this table, so it
// cannot drift from what the keys do.
type Binding struct{ Context, Keys, Desc string }

var Bindings = []Binding{
	{"Lists", "Tab", "switch between the Active and Folder/Session panels"},
	{"Lists", "↑ / ↓ / j / k", "previous or next row"},
	{"Lists", "Enter", "folder: open or close; session: open its chat (an old one is resumed)"},
	{"Lists", "z", "zoom the chat to full screen"},
	{"Lists", "f", "choose the folders shown"},
	{"Lists", "a", "account shown in the tree: all, personal, work (Active always shows all)"},
	{"Lists", "n", "new session in a folder"},
	{"Lists", "d", "session details: the last 50 events"},
	{"Lists", "/", "filter folders and sessions by text (Enter done, esc clear)"},
	{"Lists", "v", "go to the office"},
	{"Lists", "x", "close a finished nakama session"},
	{"Lists", "m", "mouse on or off (off: a plain drag copies text)"},
	{"Lists", "? / h", "this help"},
	{"Lists", "q", "quit (press twice if sessions are running)"},
	{"Chat", "Ctrl+Space", "move focus from the chat to the lists (every other key belongs to Claude)"},
	{"Chat", "F1", "help, also from inside a chat"},
	{"Zoom", "Ctrl+Space", "back to three panels (z inside the chat goes to Claude)"},
	{"Office", "Tab / ← → ↑ ↓", "select an agent"},
	{"Office", "i", "open or close the agent card"},
	{"Office", "Enter", "talk to the selected agent in the chat panel (small window: full chat)"},
	{"Office", "Ctrl+Space", "from the chat panel back to picking agents"},
	{"Office", "z", "full screen chat"},
	{"Office", "v / esc", "back to three panels"},
	{"Folder filter", "Space", "show or hide a folder"},
	{"Folder filter", "a", "show all folders"},
	{"Folder filter", "Enter / esc", "save / cancel"},
	{"New session", "type", "filter folders, or type a path (starting with / or ~)"},
	{"New session", "Tab", "change account (personal, work, ...)"},
	{"New session", "Enter / esc", "start / cancel"},
	{"Mouse", "click", "select a row, or focus a panel"},
	{"Mouse", "double click", "same as Enter"},
	{"Mouse", "wheel", "scroll the panel under the pointer"},
	{"Mouse", "Shift+drag", "copy text (while the mouse is on)"},
}
