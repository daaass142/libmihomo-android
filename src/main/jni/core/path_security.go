package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var allowedPathState struct {
	sync.RWMutex
	roots []string
}

// configureAllowedPathRoots installs the filesystem sandbox used by all
// consumer-supplied paths. homeDir is always allowed; callers may opt in
// additional app-owned roots (for example cacheDir) through InitParams.
func configureAllowedPathRoots(homeDir string, extra []string) (string, error) {
	home, err := normalizeAllowedRoot(homeDir)
	if err != nil {
		return "", fmt.Errorf("invalid home-dir: %w", err)
	}

	roots := []string{home}
	seen := map[string]struct{}{home: {}}
	for _, candidate := range extra {
		root, err := normalizeAllowedRoot(candidate)
		if err != nil {
			return "", fmt.Errorf("invalid allowed-path-root %q: %w", candidate, err)
		}
		if _, ok := seen[root]; ok {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}

	allowedPathState.Lock()
	allowedPathState.roots = roots
	allowedPathState.Unlock()
	return home, nil
}

func normalizeAllowedRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("path must be absolute")
	}
	root = filepath.Clean(root)
	if root == string(filepath.Separator) {
		return "", errors.New("filesystem root is not allowed")
	}
	return root, nil
}

// resolveAllowedPath returns a cleaned absolute path only when it is inside an
// approved root. The root itself is intentionally rejected so deleteFile can
// never remove the configured sandbox. Existing symlink components are also
// rejected to prevent lexical-prefix checks from being bypassed.
func resolveAllowedPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute")
	}
	path = filepath.Clean(path)

	allowedPathState.RLock()
	roots := append([]string(nil), allowedPathState.roots...)
	allowedPathState.RUnlock()
	if len(roots) == 0 {
		return "", errors.New("filesystem sandbox is not initialized")
	}

	for _, root := range roots {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if err := rejectSymlinkComponents(root, rel); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("path is outside approved roots: %s", path)
}

func rejectSymlinkComponents(root, rel string) error {
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path component is not allowed: %s", current)
		}
	}
	return nil
}
