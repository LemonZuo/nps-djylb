package logs

import (
	"log"
	"strings"
)

// NewStdLogger adapts the project logger to the *log.Logger that standard
// library types such as http.Server expect for their ErrorLog field. Without
// it those types write to the process-wide default logger, bypassing the
// configured level, file rotation and formatting.
//
// The output lands at warn level: net/http reports transport-level problems
// there (a TLS handshake failure, a malformed request line), which are worth
// surfacing but are usually caused by the peer rather than by nps.
func NewStdLogger(prefix string) *log.Logger {
	return log.New(&stdLogWriter{prefix: prefix}, "", 0)
}

type stdLogWriter struct {
	prefix string
}

func (w *stdLogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		Warn("%s: %s", w.prefix, msg)
	}
	return len(p), nil
}
