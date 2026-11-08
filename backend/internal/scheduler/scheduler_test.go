package scheduler_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/logger"
	"github.com/harundarat/vecrow/backend/internal/scheduler"
)

type funcTask struct {
	name string
	fn   func(context.Context) error
}

func (t funcTask) Name() string                  { return t.name }
func (t funcTask) Run(ctx context.Context) error { return t.fn(ctx) }

func TestScheduler_RunsTasksAndSurvivesFailures(t *testing.T) {
	var logs bytes.Buffer
	log, err := logger.New(logger.Config{Level: "debug", Format: "json", Output: &logs})
	if err != nil {
		t.Fatal(err)
	}

	var okRuns atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	tasks := []scheduler.Task{
		funcTask{name: "panics", fn: func(context.Context) error { panic("boom") }},
		funcTask{name: "fails", fn: func(context.Context) error { return errors.New("db down") }},
		funcTask{name: "ok", fn: func(context.Context) error {
			if okRuns.Add(1) == 2 {
				cancel()
			}
			return nil
		}},
	}

	done := make(chan error, 1)
	go func() { done <- scheduler.New(10*time.Millisecond, log, tasks...).Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop")
	}

	if okRuns.Load() < 2 {
		t.Errorf("ok task ran %d times, want >= 2 (immediately and on tick)", okRuns.Load())
	}
	out := logs.String()
	for _, want := range []string{
		`"msg":"scheduled task panicked"`,
		`"error":"db down","task":"fails"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %s:\n%s", want, out)
		}
	}
}
