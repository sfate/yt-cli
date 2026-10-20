package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Video struct {
	Title   string
	Channel string
	URL     string
}

type rawVideo struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Channel    string      `json:"channel"`
	Uploader   string      `json:"uploader"`
	URL        string      `json:"url"`
	WebpageURL string      `json:"webpage_url"`
	Entries    []*rawVideo `json:"entries"`
}

func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s)
}

func YouTubeURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	// Playlists are line-delimited: reject even encoded control characters.
	decoded, err := url.PathUnescape(s)
	if err != nil || strings.IndexFunc(decoded, unicode.IsControl) >= 0 {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "youtu.be" || host == "youtube.com" || strings.HasSuffix(host, ".youtube.com")
}

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func ParseEntries(data []byte) ([]Video, error) {
	var doc *rawVideo
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading yt-dlp results: %w", err)
	}
	if doc == nil {
		return nil, errors.New("yt-dlp returned no document")
	}
	entries := doc.Entries
	if entries == nil {
		entries = []*rawVideo{doc}
	}
	var result []Video
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		var target string
		for _, candidate := range []string{entry.WebpageURL, entry.URL} {
			if YouTubeURL(candidate) {
				target = candidate
				break
			}
		}
		if target == "" && videoID.MatchString(entry.ID) {
			target = "https://www.youtube.com/watch?v=" + entry.ID
		}
		if target == "" {
			continue
		}
		title := entry.Title
		if title == "" {
			title = target
		}
		channel := entry.Channel
		if channel == "" {
			channel = entry.Uploader
		}
		result = append(result, Video{Title: Clean(title), Channel: Clean(channel), URL: target})
	}
	return result, nil
}

func SearchArgs(query string, limit int, browser string) []string {
	target := query
	if !YouTubeURL(query) {
		target = fmt.Sprintf("ytsearch%d:%s", limit, query)
	}
	args := []string{
		"--ignore-config", "--flat-playlist", "--dump-single-json",
		"--no-warnings", "--skip-download", "--playlist-end", strconv.Itoa(limit),
		"--socket-timeout", "15", "--retries", "1",
	}
	if browser != "" {
		args = append(args, "--cookies-from-browser", browser)
	}
	return append(args, "--", target)
}

func Search(ctx context.Context, query string, limit int, browser string) ([]Video, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", SearchArgs(query, limit, browser)...)
	cmd.WaitDelay = 2 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("search failed: %w %s", err, Clean(stderr.String()))
	}
	return ParseEntries(output)
}

const bindings = `SPACE cycle pause
p cycle pause
LEFT seek -5
RIGHT seek 5
h seek -5
l seek 5
DOWN seek -30
UP seek 30
j seek -30
k seek 30
n playlist-next
N playlist-prev
> playlist-next
< playlist-prev
+ add volume 5
- add volume -5
m cycle mute
q quit
ESC quit
WHEEL_UP ignore
WHEEL_DOWN ignore
WHEEL_LEFT ignore
WHEEL_RIGHT ignore
`

// PreparePlayback creates private files; the caller must clean up after mpv exits.
func PreparePlayback(ctx context.Context, entries []Video, browser string, terminalCols, terminalRows int) (*exec.Cmd, func(), error) {
	if len(entries) == 0 {
		return nil, nil, errors.New("nothing to play")
	}
	var playlist strings.Builder
	for _, entry := range entries {
		if !YouTubeURL(entry.URL) {
			return nil, nil, errors.New("invalid YouTube URL in playlist")
		}
		fmt.Fprintln(&playlist, entry.URL)
	}
	dir, err := os.MkdirTemp("", "yt-cli-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	inputPath, playlistPath := filepath.Join(dir, "input.conf"), filepath.Join(dir, "playlist.m3u")
	for path, data := range map[string]string{
		inputPath: bindings, playlistPath: playlist.String(),
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	videoRows := max(1, terminalRows-2)
	videoCols := max(1, terminalCols)
	args := []string{
		"--no-config", "--vo=tct", "--vo-tct-algo=half-blocks",
		"--vo-tct-buffering=frame", "--vo-tct-256=no", "--profile=sw-fast", "--input-terminal=yes",
		"--input-default-bindings=no", "--input-cursor=no", "--input-conf=" + inputPath,
		fmt.Sprintf("--vo-tct-width=%d", videoCols), fmt.Sprintf("--vo-tct-height=%d", videoRows),
		"--panscan=0",
		"--term-osd=force", "--term-status-msg=${time-pos} / ${duration}",
		"--term-osd-bar=yes",
		"--video-osd=no", "--osc=no", "--ytdl=yes",
		"--ytdl-format=bestvideo[height<=360]+bestaudio/best[height<=360]/best",
		"--playlist=" + playlistPath,
	}
	if browser != "" {
		// mpv's %byte-length% escaping preserves commas in profile paths.
		args = append(args, fmt.Sprintf("--ytdl-raw-options=cookies-from-browser=%%%d%%%s", len(browser), browser))
	}
	cmd := exec.CommandContext(ctx, "mpv", args...)
	cmd.WaitDelay = 2 * time.Second
	return cmd, cleanup, nil
}
