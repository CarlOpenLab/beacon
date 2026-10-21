package agent

// Pushing this machine's diagnostic tail to the relay.
//
// The reason is mundane and important: the daemon and the desktop app are often
// started by launchd, systemd or a Windows service, with no console anywhere.
// When something goes wrong on that machine, the person who can see it is
// usually not sitting at it. A short tail posted beside the tunnel means the
// admin panel can answer "what did the Mac say" without asking anyone to open a
// terminal and copy a file.
//
// The report is a full tail, not a delta: the relay replaces what it held. That
// keeps ordering trivial and means a relay restart does not need a replay
// protocol — the next tick repairs it.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/diag"
)

// logTailLines is how much of the end of the log travels. The whole file is
// rotated at 8 MiB; a bug that is still visible was almost always written in
// the last few hundred lines, and a bounded body is what keeps a relay from
// buffering an unbounded upload.
const logTailLines = 2000

// logTailBytes bounds one upload below the relay's 2 MiB body limit. A tail is
// trimmed from the front, never the end: the newest lines are the report.
const logTailBytes = 1500 << 10

// logRequestTimeout bounds one upload. A slow relay must not leave a goroutine
// stuck on a body it no longer needs.
const logRequestTimeout = 10 * time.Second

// reportLogs pushes the tail now, then on the configured interval. It stops
// when the agent's context does. A failed upload is not retried immediately:
// the next tick carries the same tail plus whatever came after it, which is a
// better use of a link than a retry storm.
func (a *Agent) reportLogs(ctx context.Context) {
	endpoint := a.logEndpoint()
	if endpoint == "" {
		a.log.Printf("agent: diagnostic log has nowhere to go: %s", a.cfg.URL)
		return
	}
	client := &http.Client{Timeout: logRequestTimeout}
	if a.cfg.RootCAs != nil {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: a.cfg.RootCAs}}
	}
	ticker := time.NewTicker(a.cfg.LogInterval)
	defer ticker.Stop()

	var last string
	failures := 0
	for {
		if body, ok := a.logBody(); ok && body != last {
			if err := a.postLog(ctx, client, endpoint, body); err != nil {
				// An older relay has no /log endpoint, and a line every 30s
				// about a feature it does not have would be noise in the log this
				// upload is trying to be useful for. Say it once, then rarely.
				if failures == 0 || failures%20 == 0 {
					a.log.Printf("agent: log upload: %v (retrying quietly)", err)
				}
				failures++
			} else {
				last = body
				failures = 0
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// logBody renders one report. The comparison string is the encoded JSON, so a
// tail that has not grown sends nothing.
func (a *Agent) logBody() (string, bool) {
	if a.cfg.Diag == nil || a.cfg.Diag.Level() == diag.Off {
		return "", false
	}
	lines := a.cfg.Diag.Lines()
	if len(lines) == 0 {
		return "", false
	}
	if len(lines) > logTailLines {
		lines = lines[len(lines)-logTailLines:]
	}
	// Keep the end of the tail when the chosen lines are still too large.
	bytes := 0
	keep := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		bytes += len(lines[i]) + 1
		if bytes > logTailBytes {
			keep = i + 1
			break
		}
	}
	if keep < len(lines) {
		lines = lines[keep:]
	}
	// The newest line's own timestamp, not the clock now: it keeps the body
	// stable while the machine is quiet, so the reporter does not upload the
	// same tail every tick just because a time in the envelope moved.
	updated := time.Now().UTC()
	if parts := strings.SplitN(lines[len(lines)-1], " ", 2); len(parts) == 2 {
		if t, err := time.Parse("2006-01-02T15:04:05.000-07:00", parts[0]); err == nil {
			updated = t
		}
	}
	report := logReport{
		Version:   diag.BuildVersion(),
		Platform:  runtime.GOOS,
		Arch:      runtime.GOARCH,
		Level:     a.cfg.Diag.Level().String(),
		UpdatedAt: updated.Format(time.RFC3339Nano),
		Lines:     lines,
	}
	b, err := json.Marshal(report)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// logReport mirrors relay.LogReport. It is repeated rather than imported so the
// member half does not depend on the router's types; the JSON is the contract.
type logReport struct {
	Version   string   `json:"version,omitempty"`
	Platform  string   `json:"platform,omitempty"`
	Arch      string   `json:"arch,omitempty"`
	Level     string   `json:"level,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
	Lines     []string `json:"lines"`
}

func (a *Agent) postLog(ctx context.Context, client *http.Client, endpoint, body string) error {
	rctx, cancel := context.WithTimeout(ctx, logRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, endpoint, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.DeviceID+":"+a.cfg.Secret)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return &logUploadError{status: res.Status}
	}
	return nil
}

type logUploadError struct{ status string }

func (e *logUploadError) Error() string { return "relay answered " + e.status }

// logEndpoint derives the sibling /log address from the member endpoint. A
// custom path is respected: only the final /agent segment is replaced, and a
// plain directory gets /log appended.
func (a *Agent) logEndpoint() string {
	if a.cfg.LogURL != "" {
		return a.cfg.LogURL
	}
	u, err := url.Parse(a.cfg.URL)
	if err != nil || u.Host == "" {
		return ""
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	default:
		return ""
	}
	p := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(p, "/agent"):
		p = strings.TrimSuffix(p, "/agent") + "/log"
	case p == "":
		p = "/relay/log"
	default:
		p += "/log"
	}
	u.Path = p
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
