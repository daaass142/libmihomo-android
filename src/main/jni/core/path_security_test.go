package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAllowedPath(t *testing.T) {
	home := t.TempDir()
	extra := t.TempDir()
	if _, err := configureAllowedPathRoots(home, []string{extra}); err != nil {
		t.Fatalf("configure: %v", err)
	}

	inside := filepath.Join(home, "logs", "mihomo.log")
	if got, err := resolveAllowedPath(inside); err != nil || got != inside {
		t.Fatalf("inside path: got=%q err=%v", got, err)
	}

	extraFile := filepath.Join(extra, "cache.db")
	if got, err := resolveAllowedPath(extraFile); err != nil || got != extraFile {
		t.Fatalf("extra root: got=%q err=%v", got, err)
	}

	if _, err := resolveAllowedPath(home); err == nil {
		t.Fatal("sandbox root itself must be rejected")
	}

	outside := filepath.Join(filepath.Dir(home), "outside.txt")
	if _, err := resolveAllowedPath(outside); err == nil {
		t.Fatal("outside path must be rejected")
	}

	if _, err := resolveAllowedPath("relative.txt"); err == nil {
		t.Fatal("relative path must be rejected")
	}
}

func TestResolveAllowedPathRejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if _, err := configureAllowedPathRoots(home, nil); err != nil {
		t.Fatalf("configure: %v", err)
	}

	link := filepath.Join(home, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := resolveAllowedPath(filepath.Join(link, "secret")); err == nil {
		t.Fatal("symlink traversal must be rejected")
	}
}

func TestConfigureAllowedPathRootsRejectsUnsafeRoots(t *testing.T) {
	if _, err := configureAllowedPathRoots("relative", nil); err == nil {
		t.Fatal("relative home-dir must be rejected")
	}
	if _, err := configureAllowedPathRoots(string(filepath.Separator), nil); err == nil {
		t.Fatal("filesystem root must be rejected")
	}
}
