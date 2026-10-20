package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
)

type options struct {
	browser string
	limit   int
	doctor  bool
	query   string
}

func parseOptions(args []string, out io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("yt-cli", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.StringVar(&o.browser, "browser", "", "opt-in browser cookies, e.g. firefox or chrome:Profile 1")
	flags.IntVar(&o.limit, "limit", 25, "maximum results/playlist items (1–500)")
	flags.BoolVar(&o.doctor, "doctor", false, "check runtime dependencies")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(out, "Usage: yt-cli [options] [search terms or YouTube URL]\n\nOptions must precede the search query.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	if o.limit < 1 || o.limit > 500 {
		return o, errors.New("--limit must be between 1 and 500")
	}
	o.query = strings.Join(flags.Args(), " ")
	return o, nil
}

func checkDependencies(ctx context.Context) error {
	var missing []string
	for _, name := range []string{"mpv", "yt-dlp"} {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing: %s\nOn macOS: brew install mpv yt-dlp", strings.Join(missing, ", "))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return checkMPV(ctx)
}

func checkMPV(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "mpv", "--no-config", "--vo=help")
	cmd.WaitDelay = 2 * time.Second
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("mpv startup check timed out: %w\nTry running %q --no-config --vo=help directly, then retry make doctor", ctx.Err(), cmd.Path)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("mpv startup check canceled: %w", ctx.Err())
		}
		detail := ""
		if text := strings.TrimSpace(clean(string(output))); text != "" {
			detail = "\nmpv output: " + text
		}
		return fmt.Errorf("mpv startup check failed (%s): %w%s\nTry running %q --no-config --vo=help directly", cmd.Path, err, detail, cmd.Path)
	}
	if !regexp.MustCompile(`\btct\b`).Match(output) {
		return errors.New("this mpv build lacks tct video output; install a build with tct")
	}
	return nil
}

func run(args []string) error {
	o, err := parseOptions(args, os.Stderr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := checkDependencies(ctx); err != nil {
		return err
	}
	if o.doctor {
		fmt.Println("mpv (tct output) and yt-dlp found. Live YouTube playback has not been tested.")
		return nil
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return errors.New("run yt-cli in an interactive terminal or tmux pane")
	}
	if os.Getenv("COLORTERM") == "" {
		_ = os.Setenv("COLORTERM", "truecolor")
	}
	m := newModel(ctx, o)
	defer m.cancelSearch()
	_, err = tea.NewProgram(m).Run()
	return err
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "yt-cli:", err)
		os.Exit(1)
	}
}
