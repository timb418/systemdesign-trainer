package web

import (
	"fmt"
	"strings"
)

const boardSharePrefix = "Кандидат показал доску. Каноническая проекция:"

func isBoardShare(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), boardSharePrefix)
}

func boardShareDump(content string) string {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, boardSharePrefix)
	return strings.TrimSpace(s)
}

func boardShareMeta(content string) string {
	dump := boardShareDump(content)
	if dump == "" || dump == "(пустая доска)" {
		return "пустая доска"
	}
	first, _, _ := strings.Cut(dump, "\n")
	first = strings.TrimSpace(first)
	if strings.Contains(first, "узл") {
		return first
	}
	n := 1 + strings.Count(dump, "\n")
	return fmt.Sprintf("%d строк схемы", n)
}
