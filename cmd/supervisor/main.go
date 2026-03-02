package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	// How quickly a crash after startup triggers a rollback.
	crashThreshold = 10 * time.Second
	// Max consecutive rollback attempts before giving up.
	maxRollbacks = 3
)

type supervisor struct {
	srcDir  string // repo root (contains cmd/bot/)
	binDir  string // directory for compiled binaries
	binPath string // path to current binary
	prevBin string // path to previous binary (rollback target)

	mu           sync.Mutex
	child        *exec.Cmd
	childDone    chan struct{}
	childStarted time.Time

	rollbacks int // consecutive rollback count
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	srcDir, err := findRepoRoot()
	if err != nil {
		log.Fatalf("find repo root: %v", err)
	}

	binDir := filepath.Join(srcDir, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		log.Fatalf("create bin dir: %v", err)
	}

	sv := &supervisor{
		srcDir:  srcDir,
		binDir:  binDir,
		binPath: filepath.Join(binDir, "bot"),
		prevBin: filepath.Join(binDir, "bot.prev"),
	}

	// Initial build.
	log.Println("[supervisor] building bot…")
	if err := sv.build(); err != nil {
		log.Fatalf("[supervisor] initial build failed: %v", err)
	}

	// Start the bot.
	if err := sv.startChild(); err != nil {
		log.Fatalf("[supervisor] failed to start bot: %v", err)
	}

	// Signal handling.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGUSR1:
				log.Println("[supervisor] SIGUSR1 received — rebuilding…")
				sv.upgrade()
			case syscall.SIGINT, syscall.SIGTERM:
				log.Printf("[supervisor] %s received — shutting down…", sig)
				sv.stopChild()
				log.Println("[supervisor] shutdown complete.")
				os.Exit(0)
			}

		case <-sv.childDone:
			exitCode := sv.childExitCode()
			startedAt := sv.childStartTime()
			uptime := time.Since(startedAt)

			if exitCode == 0 {
				log.Println("[supervisor] bot exited cleanly (code 0). Stopping.")
				os.Exit(0)
			}

			log.Printf("[supervisor] bot crashed (code %d) after %s", exitCode, uptime.Round(time.Millisecond))

			if uptime < crashThreshold && sv.hasPrevBin() {
				sv.rollbacks++
				if sv.rollbacks > maxRollbacks {
					log.Fatalf("[supervisor] exceeded %d consecutive rollbacks — giving up", maxRollbacks)
				}
				log.Printf("[supervisor] crash within %s — rolling back (attempt %d/%d)", crashThreshold, sv.rollbacks, maxRollbacks)
				sv.rollback()
			} else {
				// Normal crash (not immediate) — just restart the same binary.
				sv.rollbacks = 0
				log.Println("[supervisor] restarting bot…")
			}

			if err := sv.startChild(); err != nil {
				log.Fatalf("[supervisor] failed to restart bot: %v", err)
			}
		}
	}
}

// build compiles the bot binary from source.
func (sv *supervisor) build() error {
	tmpBin := sv.binPath + ".new"
	cmd := exec.Command("go", "build", "-o", tmpBin, "./cmd/bot/")
	cmd.Dir = sv.srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(tmpBin)
		return fmt.Errorf("go build: %w", err)
	}

	// Rotate: current → prev, new → current.
	if _, err := os.Stat(sv.binPath); err == nil {
		if err := copyFile(sv.binPath, sv.prevBin); err != nil {
			log.Printf("[supervisor] warning: failed to backup previous binary: %v", err)
		}
	}

	if err := os.Rename(tmpBin, sv.binPath); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}

	log.Println("[supervisor] build successful")
	return nil
}

// upgrade rebuilds and restarts the bot.
func (sv *supervisor) upgrade() {
	if err := sv.build(); err != nil {
		log.Printf("[supervisor] build failed, keeping current version: %v", err)
		return
	}

	sv.rollbacks = 0
	log.Println("[supervisor] stopping current bot…")
	sv.stopChild()
	log.Println("[supervisor] starting new version…")
	if err := sv.startChild(); err != nil {
		log.Printf("[supervisor] failed to start new version: %v", err)
		if sv.hasPrevBin() {
			log.Println("[supervisor] rolling back…")
			sv.rollback()
			if err := sv.startChild(); err != nil {
				log.Fatalf("[supervisor] rollback start failed: %v", err)
			}
		}
	}
}

// rollback replaces the current binary with the previous one.
func (sv *supervisor) rollback() {
	if err := os.Rename(sv.prevBin, sv.binPath); err != nil {
		log.Printf("[supervisor] rollback rename failed: %v", err)
	}
}

func (sv *supervisor) hasPrevBin() bool {
	_, err := os.Stat(sv.prevBin)
	return err == nil
}

// startChild launches the bot binary as a child process.
func (sv *supervisor) startChild() error {
	sv.mu.Lock()
	defer sv.mu.Unlock()

	cmd := exec.Command(sv.binPath)
	cmd.Dir = sv.srcDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	sv.child = cmd
	sv.childDone = make(chan struct{})
	sv.childStarted = time.Now()

	go func() {
		cmd.Wait()
		close(sv.childDone)
	}()

	log.Printf("[supervisor] bot started (pid %d)", cmd.Process.Pid)
	return nil
}

// stopChild sends SIGTERM and waits for the child to exit (up to 10s, then SIGKILL).
func (sv *supervisor) stopChild() {
	sv.mu.Lock()
	cmd := sv.child
	done := sv.childDone
	sv.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

	select {
	case <-done:
		return
	case <-time.After(10 * time.Second):
		log.Println("[supervisor] child didn't exit in 10s, sending SIGKILL")
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
	}
}

func (sv *supervisor) childExitCode() int {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.child == nil || sv.child.ProcessState == nil {
		return -1
	}
	return sv.child.ProcessState.ExitCode()
}

func (sv *supervisor) childStartTime() time.Time {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.childStarted
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode())
}

// findRepoRoot walks up from the executable or cwd to find go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}
