// Package python provides utilities for working with the python executable.
package python

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	stashExec "github.com/stashapp/stash/pkg/exec"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/logger"
)

type Python string

func (p *Python) Command(ctx context.Context, args []string) *exec.Cmd {
	return stashExec.CommandContext(ctx, string(*p), args...)
}

// New returns a new Python instance at the given path.
func New(path string) *Python {
	ret := Python(path)
	return &ret
}

// Resolve tries to find the python executable in the system.
// It first checks for python3, then python.
// On Windows, WindowsApps aliases are tried only as a last resort.
// Returns nil and an exec.ErrNotFound error if not found.
func Resolve(configuredPythonPath string) (*Python, error) {
	if configuredPythonPath != "" {
		isFile, err := fsutil.FileExists(configuredPythonPath)
		switch {
		case err == nil && isFile:
			logger.Tracef("using configured python path: %s", configuredPythonPath)
			return New(configuredPythonPath), nil
		case err == nil && !isFile:
			logger.Warnf("configured python path is not a file: %s", configuredPythonPath)
		case err != nil:
			logger.Warnf("unable to use configured python path: %v", err)
		}
	}

	path, err := resolvePath(runtime.GOOS, exec.LookPath)
	if err != nil {
		return nil, err
	}
	return New(path), nil
}

func resolvePath(goos string, lookPath func(string) (string, error)) (string, error) {
	var fallback string
	var err error
	for _, name := range []string{"python3", "python"} {
		var path string
		path, err = lookPath(name)
		if err != nil {
			continue
		}
		if goos == "windows" && strings.Contains(strings.ToLower(strings.ReplaceAll(path, `\`, "/")), "/windowsapps/") {
			// An alias may launch a Store install or just prompt to install Python.
			// Prefer another interpreter, but keep the first alias as a fallback.
			if fallback == "" {
				fallback = path
			}
			continue
		}
		return path, nil
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("python executable not in PATH: %w", err)
}

// IsPythonCommand returns true if arg is "python" or "python3"
func IsPythonCommand(arg string) bool {
	return arg == "python" || arg == "python3"
}
