package cmd

import (
	"bytes"
	"io"
	"sync"
)

// prefixWriter writes each line with a prefix, synchronized via a mutex.
// It buffers partial lines until a newline is encountered.
type prefixWriter struct {
	w      io.Writer
	prefix string
	mu     *sync.Mutex
	buf    bytes.Buffer
}

func (pw *prefixWriter) Write(p []byte) (n int, err error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.buf.Write(p)

	for {
		line, err := pw.buf.ReadBytes('\n')
		if err != nil {
			// No complete line yet — put partial data back
			pw.buf.Write(line)
			break
		}
		pw.w.Write([]byte(pw.prefix))
		pw.w.Write(line)
	}

	return len(p), nil
}

// Flush writes any remaining buffered content (without trailing newline).
func (pw *prefixWriter) Flush() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.buf.Len() > 0 {
		pw.w.Write([]byte(pw.prefix))
		pw.w.Write(pw.buf.Bytes())
		pw.w.Write([]byte("\n"))
		pw.buf.Reset()
	}
}
