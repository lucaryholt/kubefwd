package main

import (
	"fmt"
	"os"
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

func getDebugLines() []string {
	debugLinesMu.Lock()
	defer debugLinesMu.Unlock()
	cp := make([]string, len(debugLines))
	copy(cp, debugLines)
	return cp
}
