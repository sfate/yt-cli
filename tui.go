package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type searchDone struct {
	id      int
	entries []video
	err     error
}

type playbackDone struct{ err error }

type model struct {
	ctx           context.Context
	options       options
	results       []video
	queue         []video
	selected      int
	inQueue       bool
	help          bool
	editing       bool
	query         string
	status        string
	width, height int
	searchID      int
	searchCancel  context.CancelFunc
}

func newModel(ctx context.Context, o options) *model {
	return &model{
		ctx: ctx, options: o, width: 80, height: 24,
		status: "Press / to search or paste a YouTube video/playlist URL.",
	}
}

func (m *model) Init() tea.Cmd {
	if m.options.query != "" {
		return m.startSearch(m.options.query)
	}
	return nil
}

func (m *model) entries() []video {
	if m.inQueue {
		return m.queue
	}
	return m.results
}

func (m *model) clampSelection() {
	m.selected = max(0, min(m.selected, len(m.entries())-1))
}

func (m *model) cancelSearch() {
	m.searchID++ // Ignore any result already queued by the previous command.
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
}

func (m *model) startSearch(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	m.cancelSearch()
	ctx, cancel := context.WithTimeout(m.ctx, 90*time.Second)
	m.searchCancel = cancel
	id, o := m.searchID, m.options
	m.status = "Loading " + clean(query) + " — Esc cancels"
	return func() tea.Msg {
		defer cancel()
		entries, err := search(ctx, query, o)
		return searchDone{id: id, entries: entries, err: err}
	}
}

func (m *model) play(entries []video) tea.Cmd {
	if len(entries) == 0 {
		m.status = "Nothing to play. Search or add videos to the queue first."
		return nil
	}
	cmd, cleanup, err := preparePlayback(m.ctx, entries, m.options.browser)
	if err != nil {
		m.status = "Playback failed: " + clean(err.Error())
		return nil
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// Bubble Tea releases terminal input/output until mpv exits, then restores it.
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer cleanup()
		if err != nil {
			err = fmt.Errorf("%w %s", err, clean(stderr.String()))
		}
		return playbackDone{err: err}
	})
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case searchDone:
		if msg.id != m.searchID {
			return m, nil
		}
		m.cancelSearch()
		switch {
		case errors.Is(msg.err, context.DeadlineExceeded):
			m.status = "Search timed out. Try again or check browser-session access."
		case msg.err != nil:
			m.status = clean(msg.err.Error())
		default:
			m.results, m.selected, m.inQueue = msg.entries, 0, false
			m.status = fmt.Sprintf("%d results (limit %d). Enter plays from selection.", len(m.results), m.options.limit)
		}
	case playbackDone:
		m.status = "Back from playback."
		if msg.err != nil {
			m.status = "Playback failed: " + clean(msg.err.Error())
		}
	case tea.PasteMsg:
		if m.editing {
			m.query += clean(msg.Content)
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			m.cancelSearch()
			return m, tea.Quit
		}
		if m.editing {
			switch key {
			case "enter":
				m.editing = false
				return m, m.startSearch(m.query)
			case "esc":
				m.editing = false
			case "backspace":
				runes := []rune(m.query)
				if len(runes) > 0 {
					m.query = string(runes[:len(runes)-1])
				}
			case "ctrl+u":
				m.query = ""
			default:
				m.query += clean(msg.Text)
			}
			return m, nil
		}
		switch key {
		case "q":
			m.cancelSearch()
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "esc":
			m.cancelSearch()
			m.help, m.status = false, "Ready."
		default:
			if m.help {
				return m, nil
			}
			switch key {
			case "/":
				m.editing, m.query = true, ""
			case "j", "down":
				m.selected++
			case "k", "up":
				m.selected--
			case "tab":
				m.inQueue, m.selected = !m.inQueue, 0
			case "a":
				if entries := m.entries(); len(entries) > 0 {
					m.queue = append(m.queue, entries[m.selected])
					m.status = "Added to queue."
				}
			case "A":
				if !m.inQueue {
					m.queue = append(m.queue, m.results...)
					m.status = "Added all results to queue."
				}
			case "d":
				if m.inQueue && len(m.queue) > 0 {
					m.queue = append(m.queue[:m.selected], m.queue[m.selected+1:]...)
				}
			case "P":
				return m, m.play(m.queue)
			case "enter":
				return m, m.play(m.entries()[m.selected:])
			}
		}
	}
	m.clampSelection()
	return m, nil
}

const helpText = `BROWSE
/ search or paste URL   j/k or arrows move
Enter play selection and following items
a add selection to queue   A add all results
Tab switch results/queue   d remove queued item
P play queue   ? close help   q quit

PLAYBACK (video fills this pane)
Space/p pause   h/l or left/right seek 5 seconds
j/k or down/up seek 30 seconds   +/- volume   m mute
n/N next/previous video   q/Esc return to browsing

Playlist playback advances automatically.
Browser cookies are only used with --browser.`

func (m *model) View() tea.View {
	width, height := max(1, m.width), max(1, m.height)
	lines := make([]string, height)
	mode := "SEARCH"
	if m.inQueue {
		mode = "QUEUE"
	}
	lines[0] = fmt.Sprintf("yt-cli   %s   queued: %d", mode, len(m.queue))
	visible := max(0, height-4)
	if m.help {
		for i, line := range strings.Split(helpText, "\n") {
			if i >= visible {
				break
			}
			lines[i+2] = line
		}
	} else {
		entries := m.entries()
		start := max(0, m.selected-visible+1)
		for row, index := 2, start; index < len(entries) && row < height-2; row, index = row+1, index+1 {
			entry, marker := entries[index], " "
			if index == m.selected {
				marker = ">"
			}
			lines[row] = fmt.Sprintf("%s %3d  %s  %s", marker, index+1, entry.Title, entry.Channel)
		}
	}
	if height >= 2 {
		lines[height-2] = m.status
		if m.editing {
			prompt := m.query
			if ansi.StringWidth(prompt) > max(1, width-4) {
				prompt = ansi.TruncateLeft(prompt, ansi.StringWidth(prompt)-max(1, width-4), "")
			}
			lines[height-2] = "/ " + prompt + "_"
		}
	}
	lines[height-1] = "/ search   Enter play   a queue   Tab queue   ? help   q quit"
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}
