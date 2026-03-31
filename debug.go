package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const debugMaxLines = 500

var (
	debugLines   []string
	debugLinesMu sync.Mutex
)

func debugLog(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[DEBUG] %s  %s", time.Now().Format("15:04:05.000"), msg)

	// Only print to terminal when --debug flag is set
	if debugMode {
		fmt.Fprintln(os.Stderr, line)
	}

	debugLinesMu.Lock()
	debugLines = append(debugLines, line)
	if len(debugLines) > debugMaxLines {
		debugLines = debugLines[len(debugLines)-debugMaxLines:]
	}
	debugLinesMu.Unlock()
}

// debugRunCmd logs the full command before running it, executes it with
// CombinedOutput, then logs the result. Use this for all short-lived commands.
func debugRunCmd(cmd *exec.Cmd) ([]byte, error) {
	debugLog("CMD: %s", strings.Join(cmd.Args, " "))
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if err != nil {
		if outStr != "" {
			debugLog("OUT: error=%v  output=%s", err, outStr)
		} else {
			debugLog("OUT: error=%v", err)
		}
	} else if outStr != "" {
		debugLog("OUT: %s", outStr)
	} else {
		debugLog("OUT: (ok, no output)")
	}
	return out, err
}

func getDebugLines() []string {
	debugLinesMu.Lock()
	defer debugLinesMu.Unlock()
	cp := make([]string, len(debugLines))
	copy(cp, debugLines)
	return cp
}
