package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCheckMPVTimeout(t *testing.T) {
	fakeExecutable(t, "mpv", "exec sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := checkMPV(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected actionable timeout, got %v", err)
	}
}

func TestCheckMPVDiagnostics(t *testing.T) {
	fakeExecutable(t, "mpv", "echo 'missing library' >&2\nexit 1\n")
	err := checkMPV(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing library") ||
		!strings.Contains(err.Error(), "--no-config --vo=help") {
		t.Fatalf("expected diagnostic output and reproduction command, got %v", err)
	}
}

func TestCheckMPVOutputs(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		wantError    bool
	}{
		{"supported", "tct true-color terminals", false},
		{"unsupported", "gpu GPU renderer", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeExecutable(t, "mpv", "echo '"+tc.output+"'\n")
			if err := checkMPV(context.Background()); (err != nil) != tc.wantError {
				t.Fatalf("unexpected output check result: %v", err)
			}
		})
	}
}
