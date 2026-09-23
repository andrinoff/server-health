package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

// noopUnit cannot exist, so exercising a control action against it is safe:
// systemd rejects it and nothing on the machine changes.
const noopUnit = "server-health-no-such-unit.service"

func newTestServer(t *testing.T, cfg Config) http.Handler {
	t.Helper()
	return NewServer(nil, cfg)
}

func serve(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func expectStatus(t *testing.T, got, want int, what string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got status %d, want %d", what, got, want)
	}
}

func TestHealth(t *testing.T) {
	h := newTestServer(t, Config{})
	code, body := serve(t, h, "GET", "/api/health", nil)
	expectStatus(t, code, http.StatusOK, "health")
	if body["status"] != "ok" {
		t.Fatalf("health body = %v", body)
	}
}

func TestHostSnapshot(t *testing.T) {
	h := newTestServer(t, Config{})
	code, got := serve(t, h, "GET", "/api/host", nil)
	expectStatus(t, code, http.StatusOK, "host")

	for _, key := range []string{
		"supported", "hostname", "os", "kernel", "arch", "cores", "uptime",
		"bootedAt", "load", "cpu", "memTotal", "memUsed", "memPct", "disk",
		"intervalMs", "windowMs", "series", "services",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("host snapshot missing key %q", key)
		}
	}
	if got["hostname"] == "" {
		t.Error("expected a hostname")
	}
	// series and services must be arrays, never null: the page maps over them.
	if _, ok := got["series"].([]any); !ok {
		t.Errorf("series = %#v, want an array", got["series"])
	}
	services, ok := got["services"].([]any)
	if !ok {
		t.Fatalf("services = %#v, want an array", got["services"])
	}
	if len(services) != 0 {
		t.Errorf("expected no services by default, got %d", len(services))
	}
}

func TestServiceListIsNormalizedAndUnique(t *testing.T) {
	h := newTestServer(t, Config{Services: []string{"home,caddy", " home.service "}})
	code, _ := serve(t, h, "GET", "/api/host/services", nil)
	expectStatus(t, code, http.StatusOK, "services")

	req := httptest.NewRequest("GET", "/api/host/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var services []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &services); err != nil {
		t.Fatalf("decode services: %v", err)
	}

	want := []string{"home", "caddy"}
	if len(services) != len(want) {
		t.Fatalf("got %d services, want %d: %#v", len(services), len(want), services)
	}
	for i, name := range want {
		if services[i]["name"] != name {
			t.Errorf("services[%d].name = %v, want %q", i, services[i]["name"], name)
		}
		if services[i]["unit"] != name+".service" {
			t.Errorf("services[%d].unit = %v, want %q", i, services[i]["unit"], name+".service")
		}
	}
}

func TestControlRejectsUnknownUnit(t *testing.T) {
	h := newTestServer(t, Config{Services: []string{"home,caddy"}})
	for _, path := range []string{
		"/api/host/services/sshd/restart",
		"/api/host/services/home.service.evil/restart",
		"/api/host/services/caddy%2Fextra/restart",
	} {
		code, _ := serve(t, h, "POST", path, nil)
		expectStatus(t, code, http.StatusNotFound, "POST "+path)
	}
}

func TestControlRejectsUnknownAction(t *testing.T) {
	h := newTestServer(t, Config{Services: []string{"home"}})
	for _, action := range []string{"enable", "disable", "reload", "kill"} {
		code, _ := serve(t, h, "POST", "/api/host/services/home/"+action, nil)
		expectStatus(t, code, http.StatusBadRequest, "POST action "+action)
	}
}

func TestControlOnMissingUnitFailsCleanly(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("control actions shell out to systemctl")
	}
	h := newTestServer(t, Config{Services: []string{noopUnit}})

	code, body := serve(t, h, "POST", "/api/host/services/"+noopUnit+"/start", nil)
	switch code {
	case http.StatusBadRequest, http.StatusNotFound:
		t.Fatalf("a valid request was rejected before it reached systemd: %v", body)
	case http.StatusForbidden:
		// No polkit rule in a test environment: also a correct outcome.
	case http.StatusOK:
		t.Fatal("starting a unit that does not exist must not report success")
	default:
		if body["error"] == nil {
			t.Errorf("expected an error message, got %v", body)
		}
	}
}

func TestCleanUnits(t *testing.T) {
	got := cleanUnits([]string{"home,caddy", " caddy.service ", "", ",", "backup@daily"})
	want := []string{"home.service", "caddy.service", "backup@daily.service"}
	if len(got) != len(want) {
		t.Fatalf("cleanUnits = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cleanUnits[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if units := cleanUnits(nil); len(units) != 0 {
		t.Errorf("cleanUnits(nil) = %#v, want empty", units)
	}
}

func TestDemoModeServesInventedMachine(t *testing.T) {
	h := newTestServer(t, Config{Demo: true, Services: []string{"home.service,caddy.service"}})

	code, got := serve(t, h, "GET", "/api/host", nil)
	expectStatus(t, code, http.StatusOK, "host")

	if got["supported"] != true {
		t.Errorf("demo mode should look like a supported machine, got %v", got["supported"])
	}
	if got["hostname"] != "demo-machine" {
		t.Errorf("hostname = %v, want demo-machine", got["hostname"])
	}
	points, ok := got["series"].([]any)
	if !ok || len(points) < 2 {
		t.Fatalf("expected a populated series, got %#v", got["series"])
	}
	services, ok := got["services"].([]any)
	if !ok || len(services) != 2 {
		t.Fatalf("expected 2 demo units, got %#v", got["services"])
	}
	first := services[0].(map[string]any)
	if first["activeState"] != "active" || first["mainPid"] == float64(0) {
		t.Errorf("expected the first demo unit to be running: %#v", first)
	}
}

func TestDemoModeControlSucceedsWithoutSystemd(t *testing.T) {
	h := newTestServer(t, Config{Demo: true, Services: []string{"home.service"}})

	code, body := serve(t, h, "POST", "/api/host/services/home.service/restart", nil)
	expectStatus(t, code, http.StatusOK, "demo restart")
	if body["unit"] != "home.service" {
		t.Errorf("expected the restarted unit back, got %v", body)
	}

	// The allowlist still applies in demo mode.
	code, _ = serve(t, h, "POST", "/api/host/services/sshd.service/restart", nil)
	expectStatus(t, code, http.StatusNotFound, "demo restart of an unknown unit")
}
