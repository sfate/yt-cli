# yt-cli

A keyboard-driven YouTube player for a small tmux pane. Search, queue videos,
and hand the whole pane to mpv for playback. Alacritty is the baseline: video
uses true-color Unicode half-blocks, which also work in iTerm2.

## Usage

Requires Go 1.27+ to build, mpv with `tct` output, and a current yt-dlp.
On macOS:

```sh
brew install mpv yt-dlp
make build                          # Build bin/yt-cli
make doctor                         # Check mpv and yt-dlp
make run                            # Launch the TUI
make run ARGS='ambient live'         # Start with a search
make run ARGS='--limit 100 ambient'  # Set the result limit
```

You can also run the built binary directly:

```sh
./bin/yt-cli
./bin/yt-cli 'https://www.youtube.com/playlist?list=YOUR_PLAYLIST'
```

Run these inside the tmux pane where you want the video. No graphics
passthrough is required. If tmux doesn't advertise true color, configure
its `terminal-features` for your outer terminal with the `RGB` feature.

The app is a single Go binary; Go is not needed to run a built binary.
mpv and yt-dlp must be available on `PATH`. Place options before search terms
or URLs, for example `./bin/yt-cli --limit 100 ambient live`.

## Shortcuts

| Browsing | Action |
| --- | --- |
| `/` | Search or paste a YouTube URL; Enter submits, Esc cancels |
| `j` / `k`, arrows | Select a result |
| Enter | Play selection and subsequent results/items |
| `a` / `A` | Add selected / all results to queue |
| Tab | Switch results and queue |
| `d` | Remove selected queue item |
| `P` | Play queue from the beginning |
| `?` | Help |
| Esc | Cancel an ongoing search |
| `q` | Quit |

| Playback | Action |
| --- | --- |
| Space / `p` | Pause/resume |
| `h` / `l`, left/right | Seek backward/forward 5 seconds |
| `j` / `k`, down/up | Seek backward/forward 30 seconds |
| `n` / `N` | Next/previous item |
| `+` / `-` | Volume |
| `m` | Mute |
| `q` / Esc | Return to browsing |

Playback advances automatically through the supplied list. Results and queue
remain available after playback; the queue is a session-only list, not consumed.
The default extraction limit is 25; use `--limit 100` for larger playlists
(maximum 500). Search stays cancellable while yt-dlp is running.

During playback, mpv reserves the bottom terminal row for a text progress bar
and elapsed/total time. The video is sized to the rows above it.

## Browser session

Sign in to YouTube in your normal browser, then explicitly opt in:

```sh
./bin/yt-cli --browser firefox
./bin/yt-cli --browser 'chrome:Profile 1'
```

This asks yt-dlp to read that browser's cookies for both search and playback.
The app does not collect passwords, implement OAuth, export cookies to a file,
or launch a login flow. OS/browser permissions may affect cookie access.
Public videos don't normally need a browser session. Treat browser cookies
as account credentials.

## Implementation and current limits

The root package parses command-line options and checks runtime dependencies.
`internal/app` handles terminal navigation and hands playback to mpv;
`internal/media` handles yt-dlp search, result parsing, YouTube URL validation,
and temporary mpv playlists. This keeps terminal state management, external
media tools, and process startup in separate packages.

Bubble Tea handles navigation. yt-dlp supplies metadata.
mpv owns decoding, buffering, audio synchronization, and terminal rendering.
The TUI suspends while mpv owns the pane, avoiding two competing screen writers.
Subprocesses use argument arrays, without a shell. Remote titles are stripped
of terminal control characters; temporary playlists and bindings live in a
private temporary directory and are removed after playback.

This is an initial implementation, not yet validated against live YouTube
playback in Alacritty/iTerm2. Half-block video has low resolution: a 100 × 30
cell pane provides roughly 100 × 60 color samples. Full-pane video keeps its
aspect ratio; subtitles/small text will not be practical.

YouTube extraction can require yt-dlp updates and additional runtime/token
setup. Personalized suggested-video autoplay, persistent queues, a dedicated
browser-login flow, and higher-resolution iTerm2 rendering are not implemented.
Pane resizing during playback is delegated to mpv and needs manual validation.

References: [mpv terminal output](https://mpv.io/manual/master/#video-output-drivers),
[yt-dlp](https://github.com/yt-dlp/yt-dlp),
[YouTube authentication](https://github.com/yt-dlp/yt-dlp/wiki/Extractors#logging-in-with-oauth).
