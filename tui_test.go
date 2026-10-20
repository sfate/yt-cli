package main

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func press(m *model, key rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return cmd
}

func TestQueueNavigationAndRemoval(t *testing.T) {
	m := newModel(context.Background(), options{limit: 25})
	m.results = []video{{Title: "First"}, {Title: "Second"}}
	press(m, 'a')
	press(m, 'j')
	press(m, 'a')
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.inQueue || len(m.queue) != 2 || m.queue[1].Title != "Second" {
		t.Fatalf("queue not preserved: %+v", m)
	}
	press(m, 'j')
	press(m, 'd')
	if m.selected != 0 || len(m.queue) != 1 {
		t.Fatal("selection not clamped after removing last row")
	}
	press(m, 'd')
	press(m, 'd')
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.status, "Nothing to play") {
		t.Fatal("empty queue should not launch playback")
	}
}

func TestCancelAndReplacementIgnoreStaleSearch(t *testing.T) {
	m := newModel(context.Background(), options{limit: 25})
	m.startSearch("old")
	oldID := m.searchID
	m.startSearch("new")
	m.Update(searchDone{id: oldID, entries: []video{{Title: "Stale"}}})
	if len(m.results) != 0 {
		t.Fatal("old search overwrote new search")
	}
	newID := m.searchID
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(searchDone{id: newID, entries: []video{{Title: "Canceled"}}})
	if len(m.results) != 0 || m.status != "Ready." {
		t.Fatal("canceled result changed the view")
	}
}

func TestPromptAcceptsPasteAndUnicode(t *testing.T) {
	m := newModel(context.Background(), options{limit: 25})
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "日本語\x1b\n"})
	if m.query != "日本語" {
		t.Fatalf("paste not sanitized: %q", m.query)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.query != "日本" {
		t.Fatal("backspace split a Unicode character")
	}
	press(m, 'q')
	if m.query != "日本q" {
		t.Fatal("query input triggered browse shortcut")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.editing {
		t.Fatal("Escape did not close prompt")
	}
}

func TestViewFitsSmallPane(t *testing.T) {
	m := newModel(context.Background(), options{limit: 25})
	m.results = []video{{Title: strings.Repeat("日", 100)}}
	for _, size := range [][2]int{{1, 1}, {12, 4}, {40, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > size[1] {
			t.Fatal("view exceeds pane height")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("view exceeds pane width: %q", line)
			}
		}
	}
}

func TestOptions(t *testing.T) {
	o, err := parseOptions([]string{"--browser", "chrome:Profile 1", "--limit", "100", "ambient", "live"}, io.Discard)
	if err != nil || o.query != "ambient live" || o.limit != 100 || o.browser != "chrome:Profile 1" {
		t.Fatalf("invalid parsed options: %+v, %v", o, err)
	}
	if _, err := parseOptions([]string{"--limit", "0"}, io.Discard); err == nil {
		t.Fatal("accepted invalid limit")
	}
}
