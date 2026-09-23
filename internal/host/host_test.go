package host

import (
	"testing"
	"time"
)

func TestParseCPU(t *testing.T) {
	// user nice system idle iowait irq softirq steal guest guest_nice
	line := "cpu  1000 20 500 8000 120 30 40 5 0 0"
	got, err := parseCPU(line)
	if err != nil {
		t.Fatalf("parseCPU: %v", err)
	}
	if want := 9715.0; got.total != want {
		t.Errorf("total = %v, want %v", got.total, want)
	}
	if want := 8120.0; got.idle != want {
		t.Errorf("idle = %v, want %v", got.idle, want)
	}

	if _, err := parseCPU("cpu 1 2"); err == nil {
		t.Error("expected an error for a truncated line")
	}
	if _, err := parseCPU("cpu  1 2 3 nope 5"); err == nil {
		t.Error("expected an error for a non-numeric field")
	}
}

func TestParseMemInfo(t *testing.T) {
	text := `MemTotal:        8000000 kB
MemFree:         1000000 kB
MemAvailable:    3000000 kB
Buffers:          100000 kB
SwapTotal:       2000000 kB
SwapFree:        1500000 kB
`
	got, err := parseMemInfo(text)
	if err != nil {
		t.Fatalf("parseMemInfo: %v", err)
	}
	if want := uint64(8000000 * 1024); got.total != want {
		t.Errorf("total = %d, want %d", got.total, want)
	}
	if want := uint64(3000000 * 1024); got.available != want {
		t.Errorf("available = %d, want %d", got.available, want)
	}
	if want := uint64(500000 * 1024); got.swapTotal-got.swapFree != want {
		t.Errorf("swap used = %d, want %d", got.swapTotal-got.swapFree, want)
	}

	if _, err := parseMemInfo("MemFree: 100 kB\n"); err == nil {
		t.Error("expected an error when MemTotal is missing")
	}
}

func TestParseMemInfoClampsAvailable(t *testing.T) {
	got, err := parseMemInfo("MemTotal: 100 kB\nMemAvailable: 500 kB\n")
	if err != nil {
		t.Fatalf("parseMemInfo: %v", err)
	}
	if got.available != got.total {
		t.Errorf("available = %d, want it clamped to %d", got.available, got.total)
	}
}

func TestParseUptime(t *testing.T) {
	got, err := parseUptime("12345.67 98765.43\n")
	if err != nil {
		t.Fatalf("parseUptime: %v", err)
	}
	if got != 12345.67 {
		t.Errorf("uptime = %v, want 12345.67", got)
	}
	if _, err := parseUptime("   \n"); err == nil {
		t.Error("expected an error for an empty file")
	}
}

func TestParseLoadAvg(t *testing.T) {
	got := parseLoadAvg("0.52 0.41 0.38 1/234 5678\n")
	if want := [3]float64{0.52, 0.41, 0.38}; got != want {
		t.Errorf("load = %v, want %v", got, want)
	}
	if got := parseLoadAvg("garbage"); got != [3]float64{} {
		t.Errorf("load = %v, want zeroes for a malformed file", got)
	}
}

func TestNormalizeUnit(t *testing.T) {
	cases := map[string]string{
		"caddy":         "caddy.service",
		"caddy.service": "caddy.service",
		"  home  ":      "home.service",
		"":              "",
		"backup@daily":  "backup@daily.service",
	}
	for in, want := range cases {
		if got := NormalizeUnit(in); got != want {
			t.Errorf("NormalizeUnit(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidAction(t *testing.T) {
	for _, ok := range []string{ActionStart, ActionStop, ActionRestart} {
		if !ValidAction(ok) {
			t.Errorf("ValidAction(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "enable", "disable", "status", "kill", "restart;reboot"} {
		if ValidAction(bad) {
			t.Errorf("ValidAction(%q) = true, want false", bad)
		}
	}
}

func TestParseShow(t *testing.T) {
	boot := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	out := []byte(`Description=Home manager
LoadState=loaded
ActiveState=active
SubState=running
UnitFileState=enabled
MainPID=4211
MemoryCurrent=43008000
ActiveEnterTimestampMonotonic=7360000000
`)
	svc := parseShow("home.service", out, boot)

	if svc.Name != "home" || svc.Unit != "home.service" {
		t.Errorf("name/unit = %q/%q, want home/home.service", svc.Name, svc.Unit)
	}
	if !svc.Running() {
		t.Error("expected Running() to be true for ActiveState=active")
	}
	if svc.MainPID != 4211 {
		t.Errorf("MainPID = %d, want 4211", svc.MainPID)
	}
	if svc.Memory != 43008000 {
		t.Errorf("Memory = %d, want 43008000", svc.Memory)
	}
	if want := boot.Add(7360 * time.Second); svc.Since != want.UTC().Format(time.RFC3339) {
		t.Errorf("Since = %q, want %q", svc.Since, want.UTC().Format(time.RFC3339))
	}
}

func TestParseShowToleratesMissingValues(t *testing.T) {
	// systemd prints [not set] for counters it has no value for, and a unit
	// that was never started has no enter timestamp.
	out := []byte(`LoadState=not-found
ActiveState=inactive
SubState=dead
MemoryCurrent=[not set]
ActiveEnterTimestampMonotonic=0
`)
	svc := parseShow("ghost.service", out, time.Now())
	if svc.Running() {
		t.Error("expected Running() to be false")
	}
	if svc.Memory != 0 {
		t.Errorf("Memory = %d, want 0", svc.Memory)
	}
	if svc.Since != "" {
		t.Errorf("Since = %q, want empty", svc.Since)
	}
}

func TestSamplerSeriesAndCopy(t *testing.T) {
	if !Supported() {
		t.Skip("live stats are Linux-only")
	}
	s := NewSampler(Options{Path: t.TempDir()})
	for range 3 {
		s.Sample()
	}

	snap := s.Snapshot()
	if !snap.Supported {
		t.Fatal("expected Supported on Linux")
	}
	if snap.Hostname == "" || snap.Cores == 0 || snap.Interval == 0 {
		t.Errorf("incomplete static details: %+v", snap)
	}
	if snap.MemTotal == 0 || snap.MemPct == 0 {
		t.Errorf("expected memory readings, got total=%d pct=%v", snap.MemTotal, snap.MemPct)
	}
	if snap.Uptime == 0 || snap.BootedAt == "" {
		t.Errorf("expected uptime and boot time, got %d %q", snap.Uptime, snap.BootedAt)
	}
	if snap.Disk.Total == 0 || snap.Disk.Path == "" {
		t.Errorf("expected disk readings, got %+v", snap.Disk)
	}
	if len(snap.Series) != 3 {
		t.Fatalf("series length = %d, want 3", len(snap.Series))
	}
	if snap.Series[0].T == 0 || snap.Series[0].Mem == 0 {
		t.Errorf("expected a populated point, got %+v", snap.Series[0])
	}

	// The snapshot is a copy: callers must not be able to corrupt the series.
	snap.Series[0].CPU = -1
	if next := s.Snapshot(); next.Series[0].CPU == -1 {
		t.Error("Snapshot handed out a slice aliasing the internal series")
	}
}

func TestSamplerSeriesIsCapped(t *testing.T) {
	if !Supported() {
		t.Skip("live stats are Linux-only")
	}
	s := NewSampler(Options{Path: t.TempDir()})
	for range SampleCount + 10 {
		s.Sample()
	}
	if got := len(s.Snapshot().Series); got != SampleCount {
		t.Errorf("series length = %d, want %d", got, SampleCount)
	}
}
