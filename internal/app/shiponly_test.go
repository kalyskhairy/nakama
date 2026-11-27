package app

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/input"
	"github.com/kalyskhairy/nakama/internal/monitor"
)

func TestFShowsTheShipAloneAndEnterBringsTheChatBack(t *testing.T) {
	m, _ := officeModel(t, map[string]monitor.Info{}, nil, "a")
	m.SetCellSize(10, 20)
	m.Resize(200, 45)
	m.mode = input.Home
	m.Tick(t0)
	Frame(m, 200, 45, t0)
	before := m.shipLay
	m.HandleEvent(key("f"), t0)
	f := Frame(m, 200, 45, t0)
	if !strings.Contains(f, sixelStart) || strings.Contains(ansiStrip(f), "Crew (") || strings.Contains(ansiStrip(f), "Chat ──") {
		t.Fatalf("the ship alone: %q", ansiStrip(f))
	}
	if m.shipLay.ImgW <= before.ImgW || m.shipLay.ImgW > 2*400 {
		t.Fatalf("bigger, at most twice: %d -> %d", before.ImgW, m.shipLay.ImgW)
	}
	m.HandleEvent(key("tab"), t0)
	m.HandleEvent(key("enter"), t0)
	if m.shipOnly || m.Mode() != input.Chat || !m.inOffice {
		t.Fatalf("Enter opens the chat panel: shipOnly %v mode %v inOffice %v", m.shipOnly, m.Mode(), m.inOffice)
	}
	m.leaveChat()
	m.HandleEvent(key("f"), t0)
	m.HandleEvent(key("f"), t0)
	if m.shipOnly {
		t.Fatal("f again goes back")
	}
}
