package main

import (
	"strings"
	"testing"

	"github.com/kalyskhairy/nakama/internal/app"
)

func TestToastTextIsNeverCode(t *testing.T) {
	s := toastScript(app.Notice{Title: "'; Remove-Item x #", Text: "$(calc)"})
	if strings.Contains(s, "Remove-Item") || strings.Contains(s, "calc") {
		t.Fatalf("untrusted text in the script:\n%s", s)
	}
}
