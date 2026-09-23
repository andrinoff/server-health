// Package host reports live machine health (CPU, memory, load and disk
// usage) and the state of the systemd units this dashboard may control.
//
// Readings come from /proc and statfs, so they are Linux-only. On any other
// platform Snapshot reports Supported=false and the UI hides the live panels
// instead of showing invented numbers.
package host

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// The rolling window the UI charts: a sample every SampleInterval, keeping
// SampleCount of them, so five minutes of history.
const (
	SampleInterval = 2 * time.Second
	SampleCount    = 150
)

// Point is one moment in the rolling series.
type Point struct {
	T   int64   `json:"t"`   // Unix milliseconds
	CPU float64 `json:"cpu"` // percent busy, 0-100
	Mem float64 `json:"mem"` // percent of memory in use, 0-100
}

// Disk is a filesystem usage report.
type Disk struct {
	Path      string  `json:"path"`
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Pct       float64 `json:"pct"`
}

// Snapshot is everything the Server page needs about the machine itself.
type Snapshot struct {
	Supported bool      `json:"supported"`
	Hostname  string    `json:"hostname"`
	OS        string    `json:"os"`
	Kernel    string    `json:"kernel"`
	Arch      string    `json:"arch"`
	Cores     int       `json:"cores"`
	Uptime    int64     `json:"uptime"`   // seconds since boot
	BootedAt  string    `json:"bootedAt"` // RFC3339, empty when unknown
	Load      []float64 `json:"load"`     // 1, 5 and 15 minute averages
	CPU       float64   `json:"cpu"`      // percent busy, 0-100
	MemTotal  uint64    `json:"memTotal"`
	MemUsed   uint64    `json:"memUsed"`
	MemPct    float64   `json:"memPct"`
	SwapTotal uint64    `json:"swapTotal"`
	SwapUsed  uint64    `json:"swapUsed"`
	Disk      Disk      `json:"disk"`
	Interval  int       `json:"intervalMs"`
	Window    int       `json:"windowMs"` // length of the charted series
	Series    []Point   `json:"series"`
	Error     string    `json:"error,omitempty"`
}

// Options configures a Sampler.
type Options struct {
	Path string // filesystem to report on, normally the data directory
}

type cpuTimes struct {
	idle, total float64
}

type memInfo struct {
	total, available, swapTotal, swapFree uint64
}

// Sampler keeps the rolling series and the most recent reading.
type Sampler struct {
	opts Options

	mu     sync.Mutex
	series []Point
	pcpu   cpuTimes
	prevOK bool

	cur  Snapshot
	boot time.Time

	hostname string
	osName   string
	kernel   string
	cores    int
}

// NewSampler reads the static machine details once and returns a sampler that
// is ready for Start (or for explicit Sample calls in tests).
func NewSampler(opts Options) *Sampler {
	if opts.Path == "" {
		opts.Path = "/"
	}
	s := &Sampler{opts: opts, series: []Point{}}
	s.hostname, _ = os.Hostname()
	s.osName = osPrettyName()
	s.kernel = strings.TrimSpace(readFileString("/proc/sys/kernel/osrelease"))
	s.cores = runtime.NumCPU()

	s.cur = Snapshot{
		Supported: Supported(),
		Hostname:  s.hostname,
		OS:        s.osName,
		Kernel:    s.kernel,
		Arch:      runtime.GOARCH,
		Cores:     s.cores,
		Load:      []float64{0, 0, 0},
		Interval:  int(SampleInterval / time.Millisecond),
		Window:    int(SampleInterval * SampleCount / time.Millisecond),
		Series:    s.series,
	}
	if !Supported() {
		s.cur.Error = "live machine stats read /proc, which only exists on Linux"
	}
	return s
}

// Supported reports whether live stats can be collected on this platform.
func Supported() bool { return runtime.GOOS == "linux" }

// Start samples immediately and then every SampleInterval until ctx is done.
func (s *Sampler) Start(done <-chan struct{}) {
	s.Sample()
	ticker := time.NewTicker(SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s.Sample()
		}
	}
}

// Boot returns the wall-clock time the machine booted, or the zero time when
// it cannot be determined.
func (s *Sampler) Boot() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.boot
}

// Snapshot returns the latest reading plus a copy of the series.
func (s *Sampler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.cur
	out.Series = make([]Point, len(s.series))
	copy(out.Series, s.series)
	return out
}

// Sample reads the machine once, appending a point to the series. Errors are
// recorded on the snapshot rather than raised: a failed read should not bring
// down the page.
func (s *Sampler) Sample() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !Supported() {
		return
	}

	stat, err := readCPU()
	if err != nil {
		s.cur.Error = err.Error()
		return
	}
	mem, err := readMem()
	if err != nil {
		s.cur.Error = err.Error()
		return
	}
	s.cur.Error = ""

	var cpuPct float64
	if s.prevOK && stat.total > s.pcpu.total {
		busy := (stat.total - s.pcpu.total) - (stat.idle - s.pcpu.idle)
		cpuPct = clampPct(busy / (stat.total - s.pcpu.total) * 100)
	}
	s.pcpu, s.prevOK = stat, true

	memPct := clampPct(percent(mem.total-mem.available, mem.total))
	uptime, _ := readUptime()
	load := readLoad()

	s.cur.CPU = round1(cpuPct)
	s.cur.MemTotal = mem.total
	s.cur.MemUsed = mem.total - mem.available
	s.cur.MemPct = round1(memPct)
	s.cur.SwapTotal = mem.swapTotal
	s.cur.SwapUsed = mem.swapTotal - mem.swapFree
	s.cur.Load = []float64{load[0], load[1], load[2]}
	s.cur.Uptime = int64(uptime)
	if uptime > 0 {
		s.boot = time.Now().Add(-time.Duration(uptime * float64(time.Second)))
		s.cur.BootedAt = s.boot.UTC().Format(time.RFC3339)
	}
	if d, err := diskUsage(s.opts.Path); err == nil {
		s.cur.Disk = d
	}

	s.series = append(s.series, Point{
		T:   time.Now().UnixMilli(),
		CPU: s.cur.CPU,
		Mem: s.cur.MemPct,
	})
	if len(s.series) > SampleCount {
		s.series = s.series[len(s.series)-SampleCount:]
	}
	s.cur.Series = s.series
}

// --- readers ---

func readCPU() (cpuTimes, error) {
	text := readFileString("/proc/stat")
	if text == "" {
		return cpuTimes{}, fmt.Errorf("read /proc/stat: not available")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			return parseCPU(line)
		}
	}
	return cpuTimes{}, fmt.Errorf("parse /proc/stat: no cpu line")
}

// parseCPU reads the aggregate "cpu" line of /proc/stat, in USER_HZ jiffies.
func parseCPU(line string) (cpuTimes, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return cpuTimes{}, fmt.Errorf("parse /proc/stat line %q: too few fields", line)
	}
	var t cpuTimes
	for i, raw := range fields[1:] {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("parse /proc/stat field %q: %w", raw, err)
		}
		t.total += v
		if i == 3 || i == 4 { // idle, iowait
			t.idle += v
		}
	}
	return t, nil
}

func readMem() (memInfo, error) {
	text := readFileString("/proc/meminfo")
	if text == "" {
		return memInfo{}, fmt.Errorf("read /proc/meminfo: not available")
	}
	return parseMemInfo(text)
}

// parseMemInfo reads the fields /proc/meminfo reports in kilobytes.
func parseMemInfo(text string) (memInfo, error) {
	var m memInfo
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		kb, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			m.total = kb * 1024
		case "MemAvailable":
			m.available = kb * 1024
		case "SwapTotal":
			m.swapTotal = kb * 1024
		case "SwapFree":
			m.swapFree = kb * 1024
		}
	}
	if m.total == 0 {
		return memInfo{}, fmt.Errorf("parse /proc/meminfo: no MemTotal")
	}
	if m.available > m.total {
		m.available = m.total
	}
	if m.swapFree > m.swapTotal {
		m.swapFree = m.swapTotal
	}
	return m, nil
}

func readUptime() (float64, error) {
	text := readFileString("/proc/uptime")
	if text == "" {
		return 0, fmt.Errorf("read /proc/uptime: not available")
	}
	return parseUptime(text)
}

// parseUptime reads the first field of /proc/uptime, in seconds.
func parseUptime(text string) (float64, error) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return 0, fmt.Errorf("parse /proc/uptime: empty")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/uptime %q: %w", fields[0], err)
	}
	return secs, nil
}

func readLoad() [3]float64 {
	return parseLoadAvg(readFileString("/proc/loadavg"))
}

// parseLoadAvg reads the three load averages of /proc/loadavg. A malformed
// file yields zeroes: load is decoration on the page, never a warning.
func parseLoadAvg(text string) [3]float64 {
	var out [3]float64
	fields := strings.Fields(strings.TrimSpace(text))
	for i := 0; i < 3 && i < len(fields); i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}
		}
		out[i] = v
	}
	return out
}

func osPrettyName() string {
	text := readFileString("/etc/os-release")
	for _, line := range strings.Split(text, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "PRETTY_NAME="); ok {
			return strings.Trim(name, `"`)
		}
	}
	return runtime.GOOS
}

func diskUsage(path string) (Disk, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Disk{}, fmt.Errorf("statfs %s: %w", path, err)
	}
	blockSize := uint64(st.Bsize)
	total := st.Blocks * blockSize
	available := st.Bavail * blockSize
	if available > total {
		available = total
	}
	used := total - available
	return Disk{
		Path:      path,
		Total:     total,
		Used:      used,
		Available: available,
		Pct:       round1(percent(used, total)),
	}, nil
}

// --- helpers ---

func readFileString(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return float64(int64(v*10+0.5)) / 10
}
