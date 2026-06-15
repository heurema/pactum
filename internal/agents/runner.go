package agents

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// lockedWriter serializes concurrent writes to an underlying writer. os/exec
// copies a command's stdout and stderr on separate goroutines, so a live writer
// shared by both streams must be guarded.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type activityWriter struct {
	w        io.Writer
	activity chan<- struct{}
}

func (w activityWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		select {
		case w.activity <- struct{}{}:
		default:
		}
	}
	return w.w.Write(p)
}

const completedDespiteTimeoutNotice = "idle timeout fired after the agent completed; treating as completed with warning"

// finalizeTimedOutAttempt resolves an idle-killed attempt against the agent's
// completion signal: when the captured stdout carries a successful terminal
// marker (or the caller already observed completion, e.g. a recorded ACP
// prompt response), the attempt is finalized as completed-with-warning — exit
// code 0, the warning appended to the attempt stderr and the live writer.
// TimedOut stays true on the result for the record. Without the marker the
// timed-out failure stands (exit -1).
// An empty stdoutPath skips the captured-output detection: over ACP the
// attempt log is free-streamed agent text where the CLI terminal markers
// cannot legitimately appear (an agent merely QUOTING one must not convert a
// stalled turn into success), so the recorded prompt response is the
// protocol's only completion signal.
func finalizeTimedOutAttempt(agent AgentDescriptor, stdoutPath string, alreadyCompleted bool, stderr io.Writer, live io.Writer) (exitCode int, completed bool) {
	completed = alreadyCompleted
	if !completed && stdoutPath != "" {
		stdout, err := os.ReadFile(stdoutPath)
		completed = err == nil && agentRunCompleted(agent, stdout)
	}
	if !completed {
		return -1, false
	}
	line := completedDespiteTimeoutNotice + "\n"
	_, _ = io.WriteString(stderr, line)
	if live != nil {
		_, _ = io.WriteString(live, line)
	}
	return 0, true
}

func writeUsageWarning(usage TokenUsage, stderr io.Writer, live io.Writer) {
	if strings.TrimSpace(usage.CaptureWarning) == "" {
		return
	}
	line := "usage capture warning: " + usage.CaptureWarning + "\n"
	_, _ = io.WriteString(stderr, line)
	if live != nil {
		_, _ = io.WriteString(live, line)
	}
}

func startIdleTimeout(timeout time.Duration, activity <-chan struct{}, cancel context.CancelFunc, timedOut *atomic.Bool) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case <-timer.C:
				timedOut.Store(true)
				cancel()
				return
			case <-activity:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(timeout)
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}
