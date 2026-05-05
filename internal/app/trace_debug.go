package app

import (
	"fmt"
	"os"
	"time"
)

// traceLog appends a timestamped line to ./tira-trace.log.
// Temporary debugging aid — remove once the E hotkey issue is diagnosed.
func traceLog(format string, args ...any) {
	f, err := os.OpenFile("/tmp/tira-trace.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}
