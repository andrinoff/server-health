// Package demo invents readings and units so the page can be worked on
// somewhere that is not Linux (a laptop, a CI box, a design pass).
//
// It exists only behind the -demo flag: nothing here runs on a real server,
// and the invented machine always calls itself demo-machine so it cannot be
// mistaken for the one you are watching.
package demo

import (
	"fmt"
	"math"
	"time"

	"github.com/andrinoff/server-health/internal/host"
)

const (
	window   = 5 * time.Minute
	step     = 2 * time.Second
	samples  = 150
	hostname = "demo-machine"
)

var bootedAt = time.Date(2026, 9, 11, 4, 12, 0, 0, time.UTC)

// Snapshot returns a moving five-minute trace: the values are derived from the
// clock, so each poll shows a live machine rather than a frozen fixture.
func Snapshot() host.Snapshot {
	now := time.Now()

	series := make([]host.Point, 0, samples)
	for i := samples - 1; i >= 0; i-- {
		at := now.Add(-time.Duration(i) * step)
		seconds := float64(at.UnixMilli()) / 1000
		cpu := 13 + 15*math.Sin(seconds/23) + 6*math.Sin(seconds/7)
		mem := 42 + 4*math.Sin(seconds/61)
		series = append(series, host.Point{
			T:   at.UnixMilli(),
			CPU: round1(clamp(cpu, 1, 99)),
			Mem: round1(clamp(mem, 1, 99)),
		})
	}
	last := series[len(series)-1]

	const memTotal = uint64(8) << 30
	const diskTotal = uint64(40) << 30
	const diskUsed = uint64(21) << 30

	return host.Snapshot{
		Supported: true,
		Hostname:  hostname,
		OS:        "Ubuntu 24.04.2 LTS",
		Kernel:    "6.8.0-45-generic",
		Arch:      "amd64",
		Cores:     4,
		Uptime:    int64(now.Sub(bootedAt).Seconds()),
		BootedAt:  bootedAt.Format(time.RFC3339),
		Load:      []float64{0.34, 0.21, 0.18},
		CPU:       last.CPU,
		MemTotal:  memTotal,
		MemUsed:   uint64(float64(memTotal) * last.Mem / 100),
		MemPct:    last.Mem,
		SwapTotal: 2 << 30,
		SwapUsed:  46 << 20,
		Disk: host.Disk{
			Path:      "/",
			Total:     diskTotal,
			Used:      diskUsed,
			Available: diskTotal - diskUsed,
			Pct:       round1(float64(diskUsed) / float64(diskTotal) * 100),
		},
		Interval: int(step / time.Millisecond),
		Window:   int(window / time.Millisecond),
		Series:   series,
	}
}

// Services returns plausible states: the first two units up, the third down.
func Services(names []string) []host.Service {
	uptime := time.Since(bootedAt)
	out := make([]host.Service, 0, len(names))
	for i, name := range names {
		unit := host.NormalizeUnit(name)
		svc := host.Service{
			Name:        trimUnit(unit),
			Unit:        unit,
			Description: describedUnit(unit),
			LoadState:   "loaded",
			ActiveState: "active",
			SubState:    "running",
			Enabled:     "enabled",
			MainPID:     4211 + i*907,
			Memory:      uint64(43_000_000 + i*11_500_000),
			Since:       time.Now().Add(-(uptime - time.Duration(i+1)*time.Hour)).UTC().Format(time.RFC3339),
		}
		if i%3 == 2 {
			svc.ActiveState = "inactive"
			svc.SubState = "dead"
			svc.MainPID = 0
			svc.Memory = 0
			svc.Since = ""
		}
		out = append(out, svc)
	}
	return out
}

// Action pretends to act, so the buttons and the confirmation flow can be used
// without a machine to change.
func Action(unit, action string, timeout time.Duration) error {
	if !host.ValidAction(action) {
		return fmt.Errorf("unsupported action %q", action)
	}
	time.Sleep(400 * time.Millisecond)
	return nil
}

func describedUnit(unit string) string {
	switch unit {
	case "caddy.service":
		return "Caddy web server"
	case "server-health.service":
		return "Server health dashboard"
	default:
		return "Demo unit"
	}
}

func trimUnit(unit string) string {
	if len(unit) > len(".service") {
		return unit[:len(unit)-len(".service")]
	}
	return unit
}

func clamp(v, low, high float64) float64 {
	return math.Min(math.Max(v, low), high)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
