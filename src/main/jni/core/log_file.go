//go:build android && cgo

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/metacubex/mihomo/log"
)

const (
	fileSinkMaxBytes = 5 * 1024 * 1024 // 5 MB per file
	fileSinkMaxCount = 5               // .log + .1..(N-1) — 5 files total
	fileSinkBufBytes = 8 * 1024        // bufio.Writer flushes on Close + 500ms tick
	crashFileName    = "crash.log"
)

// State owned by dispatchLoop + fileSinkMu; touch only via helpers below.
var (
	fileSinkMu   sync.Mutex
	fileSinkPath string
	fileSink     *os.File
	fileSinkWr   *bufio.Writer
	fileSinkSize int64
	crashOutput  *os.File
)

func setLogFilePathImpl(path string) error {
	fileSinkMu.Lock()
	defer fileSinkMu.Unlock()
	if path == "" {
		closeFileSinkLocked()
		fileSinkPath = ""
		return nil
	}

	safePath, err := resolveAllowedPath(path)
	if err != nil {
		return err
	}
	if fileSinkPath == safePath {
		return nil
	}
	closeFileSinkLocked()
	if err := os.MkdirAll(filepath.Dir(safePath), 0o700); err != nil {
		return err
	}
	if err := openFileSinkLocked(safePath); err != nil {
		return err
	}
	fileSinkPath = safePath
	setCrashOutputLocked(filepath.Join(filepath.Dir(safePath), crashFileName))
	return nil
}

// A fatal signal makes the runtime write its traceback to fd 2, which
// captureStdFd has redirected into a pipe drained by a goroutine in this very
// process — so a crash killed its own reader and left no trace. SetCrashOutput
// hands the runtime a real file instead, which survives the process.
func setCrashOutputLocked(path string) {
	safePath, err := resolveAllowedPath(path)
	if err != nil {
		log.Warnln("[APP] crash output path: %v", err)
		return
	}
	f, err := os.OpenFile(safePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		log.Warnln("[APP] crash output %s: %v", safePath, err)
		return
	}
	if err := f.Chmod(0o600); err != nil {
		log.Warnln("[APP] crash output chmod %s: %v", safePath, err)
		_ = f.Close()
		return
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		log.Warnln("[APP] SetCrashOutput: %v", err)
		_ = f.Close()
		return
	}
	if crashOutput != nil {
		_ = crashOutput.Close()
	}
	crashOutput = f
}

func openFileSinkLocked(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	info, statErr := f.Stat()
	if statErr == nil {
		fileSinkSize = info.Size()
	} else {
		fileSinkSize = 0
	}
	fileSink = f
	fileSinkWr = bufio.NewWriterSize(f, fileSinkBufBytes)
	return nil
}

func closeFileSinkLocked() {
	if fileSinkWr != nil {
		_ = fileSinkWr.Flush()
		fileSinkWr = nil
	}
	if fileSink != nil {
		_ = fileSink.Sync()
		_ = fileSink.Close()
		fileSink = nil
	}
	fileSinkSize = 0
}

func writeFileSink(e dispatchEvent) {
	fileSinkMu.Lock()
	defer fileSinkMu.Unlock()
	if fileSinkWr == nil {
		return
	}
	line := fmt.Sprintf("%s [%s] [%s] %s\n",
		e.when.Format("2006-01-02T15:04:05.000000000Z07:00"),
		levelTag(e.level),
		e.tag,
		e.payload,
	)
	n, err := fileSinkWr.WriteString(line)
	if err != nil {
		// Disable on write failure so logcat keeps flowing.
		closeFileSinkLocked()
		fileEnabled.Store(false)
		return
	}
	fileSinkSize += int64(n)
	if fileSinkSize >= fileSinkMaxBytes {
		rotateFileSinkLocked()
	}
}

func flushFileSink() {
	fileSinkMu.Lock()
	defer fileSinkMu.Unlock()
	if fileSinkWr != nil {
		_ = fileSinkWr.Flush()
	}
}

func rotateFileSinkLocked() {
	if fileSink == nil {
		return
	}
	_ = fileSinkWr.Flush()
	_ = fileSink.Sync()
	_ = fileSink.Close()
	fileSink = nil
	fileSinkWr = nil

	base := fileSinkPath
	_ = os.Remove(rotName(base, fileSinkMaxCount-1))
	for i := fileSinkMaxCount - 2; i >= 1; i-- {
		_ = os.Rename(rotName(base, i), rotName(base, i+1))
	}
	_ = os.Rename(base, rotName(base, 1))
	if err := openFileSinkLocked(base); err != nil {
		fileEnabled.Store(false)
		fileSinkSize = 0
	}
}

func rotName(base string, n int) string {
	return fmt.Sprintf("%s.%d", base, n)
}

func levelTag(l log.LogLevel) string {
	switch l {
	case log.DEBUG:
		return "D"
	case log.INFO:
		return "I"
	case log.WARNING:
		return "W"
	case log.ERROR:
		return "E"
	case log.SILENT:
		return "S"
	default:
		return "I"
	}
}
