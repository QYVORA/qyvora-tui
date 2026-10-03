package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// exportSession writes every execution's retained tool output and structured
// event data to a new log in the current directory. Transcript display limits
// do not apply to this file.
func (m model) exportSession() (string, error) {
	name := safeFilePart(m.cfg.Title)
	if name == "" {
		name = "qyvora"
	}
	path := fmt.Sprintf("%s-session-%s.log", name, time.Now().UTC().Format("20060102T150405.000000000Z"))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	failed := true
	defer func() {
		_ = f.Close()
		if failed {
			_ = os.Remove(path)
		}
	}()

	w := bufio.NewWriter(f)
	for _, b := range m.blocks {
		if _, err = fmt.Fprintf(w, "=== %s | %s | %s ===\n", b.command, b.started.Format(time.RFC3339), b.status); err != nil {
			return "", err
		}
		if len(b.rawOutput) > 0 {
			if _, err = fmt.Fprintln(w, "--- tool output ---"); err != nil {
				return "", err
			}
			for _, line := range b.rawOutput {
				if _, err = fmt.Fprintln(w, line); err != nil {
					return "", err
				}
			}
		}
		if len(b.rows) > 0 {
			if _, err = fmt.Fprintln(w, "--- events (JSONL) ---"); err != nil {
				return "", err
			}
			for _, row := range b.rows {
				ev := Event{Timestamp: row.At, Level: row.Level, Event: row.Type, Data: row.Data}
				var encoded []byte
				encoded, err = json.Marshal(ev)
				if err != nil {
					return "", err
				}
				if _, err = w.Write(encoded); err != nil {
					return "", err
				}
				if err = w.WriteByte('\n'); err != nil {
					return "", err
				}
			}
		}
		if _, err = fmt.Fprintln(w); err != nil {
			return "", err
		}
	}
	if err = w.Flush(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	failed = false
	return path, nil
}

func safeFilePart(s string) string {
	if i := strings.LastIndexAny(s, "/\\"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
