package web

import (
	"strings"
	"testing"
)

func TestBoardShareHelpers(t *testing.T) {
	msg := boardSharePrefix + "\n12 узлов, 16 связей\n\nУзлы:\n• сервис — API"
	if !isBoardShare(msg) {
		t.Fatal("expected board share")
	}
	if isBoardShare("обычный ответ") {
		t.Fatal("plain text is not a board share")
	}
	if got := boardShareMeta(msg); got != "12 узлов, 16 связей" {
		t.Fatalf("meta: %q", got)
	}
	dump := boardShareDump(msg)
	if !strings.Contains(dump, "Узлы:") {
		t.Fatalf("dump: %q", dump)
	}
}

func TestBoardShareLegacyDump(t *testing.T) {
	msg := boardSharePrefix + "\nA --> B\nB --> C"
	if got := boardShareMeta(msg); got != "2 строк схемы" {
		t.Fatalf("legacy meta: %q", got)
	}
}
