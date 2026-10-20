package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseEntries(t *testing.T) {
	data := []byte(`{"entries":[null,
		{"id":"abcdefghijk","title":"Track\n\u001b]52;clipboard\u0007","uploader":"Artist"},
		{"id":"../../bad","url":"https://evil.example/video"},
		{"url":"https://www.youtube.com/watch?v=12345678901","title":"Second"}]}`)
	entries, err := ParseEntries(data)
	if err != nil || len(entries) != 2 {
		t.Fatalf("got %v, %v", entries, err)
	}
	if entries[0].URL != "https://www.youtube.com/watch?v=abcdefghijk" || entries[0].Channel != "Artist" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
	if strings.ContainsAny(entries[0].Title, "\x1b\n\x07") {
		t.Fatal("terminal controls survived sanitization")
	}
}

func TestParseSingleAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		input string
		count int
	}{
		{`{"id":"abcdefghijk"}`, 1},
		{`{"entries":[]}`, 0},
	} {
		entries, err := ParseEntries([]byte(tc.input))
		if err != nil || len(entries) != tc.count {
			t.Fatalf("%s: got %v, %v", tc.input, entries, err)
		}
	}
	for _, input := range []string{"null", "not json", `{"entries":"invalid"}`} {
		if _, err := ParseEntries([]byte(input)); err == nil {
			t.Errorf("accepted invalid document %q", input)
		}
	}
}

func TestParseMetadataFallbackAndValidation(t *testing.T) {
	data := []byte(`{"entries":[{"id":"abcdefghijk","webpage_url":"https://youtu.be/abcdefghijk","title":"","channel":""},{"id":"bad_id"},{"id":"12345678901","title":"Video","uploader":"Uploader"}]}`)
	entries, err := ParseEntries(data)
	if err != nil || len(entries) != 2 {
		t.Fatalf("got %v, %v", entries, err)
	}
	if entries[0].Title != entries[0].URL {
		t.Fatalf("missing title did not fall back to URL: %+v", entries[0])
	}
	if entries[1].Channel != "Uploader" {
		t.Fatalf("missing channel did not fall back to uploader: %+v", entries[1])
	}
}

func TestYouTubeURL(t *testing.T) {
	for _, target := range []string{
		"https://youtube.com.evil.test/watch?v=x", "file:///tmp/x",
		"https://youtube.com@evil.test/x", "https://youtube.com/x%0ahttps://evil.test",
	} {
		if YouTubeURL(target) {
			t.Errorf("accepted %q", target)
		}
	}
	if !YouTubeURL("https://youtu.be/abcdefghijk") {
		t.Fatal("rejected valid short URL")
	}
	for _, target := range []string{
		"http://youtube.com/watch?v=abcdefghijk",
		"https://m.youtube.com/watch?v=abcdefghijk",
		"https://YOUTUBE.COM/watch?v=abcdefghijk",
	} {
		if !YouTubeURL(target) {
			t.Errorf("rejected valid URL %q", target)
		}
	}
}

func TestSearchArgsLiteralQueryAndBrowser(t *testing.T) {
	query := `music; $(touch /tmp/not-real) "quoted"`
	args := SearchArgs(query, 25, "")
	if !reflect.DeepEqual(args[len(args)-2:], []string{"--", "ytsearch25:" + query}) {
		t.Fatalf("query was not passed literally: %q", args)
	}
	url := "https://www.youtube.com/playlist?list=PL123"
	args = SearchArgs(url, 100, "chrome:Profile 1")
	if args[len(args)-1] != url || args[len(args)-3] != "chrome:Profile 1" {
		t.Fatalf("unexpected playlist/browser arguments: %q", args)
	}
}

func TestPreparePlayback(t *testing.T) {
	entries := []Video{{URL: "https://youtu.be/abcdefghijk"}, {URL: "https://youtu.be/12345678901"}}
	browser := "firefox:profile,é"
	cmd, cleanup, err := PreparePlayback(context.Background(), entries, browser, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var playlist string
	for _, arg := range cmd.Args {
		if strings.HasPrefix(arg, "--playlist=") {
			playlist = strings.TrimPrefix(arg, "--playlist=")
		}
	}
	data, err := os.ReadFile(playlist)
	if err != nil || string(data) != entries[0].URL+"\n"+entries[1].URL+"\n" {
		t.Fatalf("playlist order/content: %q, %v", data, err)
	}
	conf, err := os.ReadFile(filepath.Join(filepath.Dir(playlist), "input.conf"))
	if err != nil || !strings.Contains(string(conf), "SPACE cycle pause") ||
		!strings.Contains(string(conf), "ESC quit") ||
		!strings.Contains(string(conf), "WHEEL_UP ignore") ||
		!strings.Contains(string(conf), "WHEEL_DOWN ignore") {
		t.Fatalf("missing bindings: %s, %v", conf, err)
	}
	args := strings.Join(cmd.Args, "\n")
	if !strings.Contains(args, "--vo=tct") ||
		!strings.Contains(args, "--input-cursor=no") ||
		!strings.Contains(args, "--vo-tct-width=100") ||
		!strings.Contains(args, "--vo-tct-height=28") ||
		!strings.Contains(args, "--panscan=0") ||
		!strings.Contains(args, "--term-osd=force") ||
		!strings.Contains(args, "--term-status-msg=${time-pos} / ${duration}") ||
		!strings.Contains(args, "--term-osd-bar=yes") ||
		!strings.Contains(args, "--video-osd=no") ||
		strings.Contains(args, "--osd-level=0") ||
		!strings.Contains(args, "--ytdl-raw-options=cookies-from-browser=%18%"+browser) {
		t.Fatalf("missing renderer or incorrect byte-length escaping: %s", args)
	}
	info, err := os.Stat(playlist)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("playlist permissions: %v, %v", info, err)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(playlist)); !os.IsNotExist(err) {
		t.Fatalf("temporary directory survived cleanup: %v", err)
	}
}

func TestPreparePlaybackRejectsInvalidQueue(t *testing.T) {
	for _, entries := range [][]Video{nil, {{URL: "https://youtube.com/x\nfile:///tmp/x"}}} {
		if _, _, err := PreparePlayback(context.Background(), entries, "", 80, 24); err == nil {
			t.Fatal("accepted empty or invalid queue")
		}
	}
}

func fakeExecutable(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSearchSubprocessAndCancellation(t *testing.T) {
	fakeExecutable(t, "yt-dlp", `printf '%s' '{"entries":[{"id":"abcdefghijk","title":"Track"}]}'`)
	entries, err := Search(context.Background(), "music", 25, "")
	if err != nil || len(entries) != 1 {
		t.Fatalf("subprocess search failed: %v, %v", entries, err)
	}
	fakeExecutable(t, "yt-dlp", "exec sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = Search(ctx, "music", 25, "")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("search failed to cancel promptly: %v", err)
	}
}

func TestSearchSubprocessFailureIncludesSanitizedError(t *testing.T) {
	fakeExecutable(t, "yt-dlp", "echo 'request failed' >&2; exit 7\n")
	_, err := Search(context.Background(), "music", 25, "")
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("search failure omitted yt-dlp diagnostic: %v", err)
	}
}
