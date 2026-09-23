package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Actions the dashboard is allowed to perform on a unit.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"
)

// ErrPrivileges means systemd understood the request but refused it for lack
// of permission. The API turns this into an actionable message.
var ErrPrivileges = errors.New("systemd refused the action")

// Service is the state of one systemd unit.
type Service struct {
	Name        string `json:"name"` // as configured, without the .service suffix
	Unit        string `json:"unit"`
	Description string `json:"description"`
	LoadState   string `json:"loadState"`   // loaded | not-found | masked
	ActiveState string `json:"activeState"` // active | inactive | failed | activating
	SubState    string `json:"subState"`    // running | exited | dead | failed
	Enabled     string `json:"enabled"`     // enabled | disabled | static
	MainPID     int    `json:"mainPid"`
	Memory      uint64 `json:"memory"` // bytes, 0 when systemd reports none
	Since       string `json:"since"`  // RFC3339, empty when never started
	Error       string `json:"error,omitempty"`
}

// Running reports whether the unit's main process is up.
func (s Service) Running() bool { return s.ActiveState == "active" }

// NormalizeUnit accepts "caddy" or "caddy.service" and always returns the
// fully qualified unit name.
func NormalizeUnit(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, ".service") {
		return name
	}
	return name + ".service"
}

// ValidAction reports whether action is one of the three we expose.
func ValidAction(action string) bool {
	switch action {
	case ActionStart, ActionStop, ActionRestart:
		return true
	}
	return false
}

// Services queries each unit, in order. A unit that cannot be read keeps its
// place in the list with the problem recorded on it, so the page shows a row
// with an explanation instead of silently dropping it. Units are queried at
// the same time, so one slow systemctl does not delay the others.
func Services(names []string, boot time.Time) []Service {
	out := make([]Service, len(names))
	var wg sync.WaitGroup
	for i, name := range names {
		unit := NormalizeUnit(name)
		if unit == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc, err := Query(unit, boot)
			if err != nil {
				svc = Service{Name: strings.TrimSuffix(unit, ".service"), Unit: unit, Error: err.Error()}
			}
			out[i] = svc
		}()
	}
	wg.Wait()
	return out
}

// Query reads a unit's state with systemctl show, which every user may run.
func Query(unit string, boot time.Time) (Service, error) {
	if !Supported() || unit == "" {
		return Service{}, fmt.Errorf("systemd is only available on Linux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "show", unit,
		"-p", "Description",
		"-p", "LoadState",
		"-p", "ActiveState",
		"-p", "SubState",
		"-p", "UnitFileState",
		"-p", "MainPID",
		"-p", "MemoryCurrent",
		"-p", "ActiveEnterTimestampMonotonic",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) && ctx.Err() == nil {
			if msg := firstLine(stderr.String()); msg != "" {
				return Service{}, fmt.Errorf("systemctl show %s: %s", unit, msg)
			}
		}
		return Service{}, fmt.Errorf("systemctl show %s: %w", unit, err)
	}
	return parseShow(unit, out, boot), nil
}

// parseShow turns the key=value output of systemctl show into a Service.
func parseShow(unit string, out []byte, boot time.Time) Service {
	svc := Service{Name: strings.TrimSuffix(unit, ".service"), Unit: unit}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Description":
			svc.Description = value
		case "LoadState":
			svc.LoadState = value
		case "ActiveState":
			svc.ActiveState = value
		case "SubState":
			svc.SubState = value
		case "UnitFileState":
			svc.Enabled = value
		case "MainPID":
			svc.MainPID, _ = strconv.Atoi(value)
		case "MemoryCurrent":
			if n, err := strconv.ParseUint(value, 10, 64); err == nil {
				svc.Memory = n
			}
		case "ActiveEnterTimestampMonotonic":
			if usec, err := strconv.ParseInt(value, 10, 64); err == nil && usec > 0 && !boot.IsZero() {
				svc.Since = boot.Add(time.Duration(usec) * time.Microsecond).UTC().Format(time.RFC3339)
			}
		}
	}
	return svc
}

// Action starts, stops or restarts a unit. Privileges are never escalated
// here: if the service user is not permitted, the caller gets ErrPrivileges
// and the UI explains how to grant access.
func Action(unit, action string, timeout time.Duration) error {
	if !ValidAction(action) {
		return fmt.Errorf("unsupported action %q", action)
	}
	if !Supported() {
		return fmt.Errorf("systemd is only available on Linux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", action, unit)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := firstLine(stderr.String())
		if ctx.Err() != nil {
			return fmt.Errorf("systemctl %s %s did not finish within %s", action, unit, timeout)
		}
		if isPrivilegeError(msg) {
			return fmt.Errorf("%w: %s", ErrPrivileges, msg)
		}
		if msg != "" {
			return fmt.Errorf("systemctl %s %s: %s", action, unit, msg)
		}
		return fmt.Errorf("systemctl %s %s: %w", action, unit, err)
	}
	return nil
}

func isPrivilegeError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, needle := range []string{
		"interactive authentication required",
		"access denied",
		"permission denied",
		"not authorized",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(s, "Failed to ")
}
