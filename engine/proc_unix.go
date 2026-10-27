package main

import (
	"os/exec"
	"runtime"
	"syscall"
)

// detachSelf moves this process into its own session.
func detachSelf() { syscall.Setsid() }

// openPaths opens files/folders with their default app (Finder …).
func openPaths(paths []string) {
	if runtime.GOOS == "darwin" && len(paths) > 0 {
		exec.Command("/usr/bin/open", paths...).Start()
	}
}

// revealPaths shows the files selected in Finder.
func revealPaths(paths []string) {
	if runtime.GOOS == "darwin" && len(paths) > 0 {
		exec.Command("/usr/bin/open", append([]string{"-R"}, paths...)...).Start()
	}
}
