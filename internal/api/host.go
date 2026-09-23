package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/andrinoff/server-health/internal/demo"
	"github.com/andrinoff/server-health/internal/host"
)

// controlTimeout bounds how long a start/stop/restart may take before we give
// up and tell the user, rather than leaving the button spinning forever.
const controlTimeout = 20 * time.Second

// payload is a machine snapshot plus the unit states, so a single request
// keeps the whole page consistent.
type payload struct {
	host.Snapshot
	Services []host.Service `json:"services"`
}

// read gathers the machine snapshot and the unit states. In demo mode both are
// invented, so the same handlers serve a preview on a laptop.
func (s *Server) read() (host.Snapshot, []host.Service) {
	if s.demo {
		return demo.Snapshot(), demo.Services(s.services)
	}
	return s.host.Snapshot(), host.Services(s.services, s.host.Boot())
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	snapshot, services := s.read()
	writeJSON(w, http.StatusOK, payload{Snapshot: snapshot, Services: services})
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	_, services := s.read()
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) controlService(w http.ResponseWriter, r *http.Request) {
	requested := r.PathValue("name")
	unit, ok := s.unitFor(requested)
	if !ok {
		writeError(w, http.StatusNotFound, "this dashboard is not configured to manage "+requested)
		return
	}
	action := r.PathValue("action")
	if !host.ValidAction(action) {
		writeError(w, http.StatusBadRequest, "action must be start, stop or restart")
		return
	}

	run := host.Action
	if s.demo {
		run = demo.Action
	}
	if err := run(unit, action, controlTimeout); err != nil {
		if errors.Is(err, host.ErrPrivileges) {
			writeError(w, http.StatusForbidden, "the service user is not allowed to "+action+" "+unit+
				". Run `sudo deploy/service-control.sh "+strings.Join(s.services, " ")+"` on the server to grant it.")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	_, services := s.read()
	for _, svc := range services {
		if svc.Unit == unit {
			writeJSON(w, http.StatusOK, svc)
			return
		}
	}
	writeJSON(w, http.StatusOK, host.Service{Name: strings.TrimSuffix(unit, ".service"), Unit: unit})
}

// unitFor reports whether name is one of the units this dashboard was told to
// manage, so a typo in the URL can never reach systemd.
func (s *Server) unitFor(name string) (string, bool) {
	want := host.NormalizeUnit(name)
	if want == "" {
		return "", false
	}
	for _, unit := range s.services {
		if unit == want {
			return want, true
		}
	}
	return "", false
}

// cleanUnits normalizes a configured unit list and drops blanks and repeats.
func cleanUnits(names []string) []string {
	units := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, raw := range names {
		for _, part := range strings.Split(raw, ",") {
			unit := host.NormalizeUnit(part)
			if unit == "" || seen[unit] {
				continue
			}
			seen[unit] = true
			units = append(units, unit)
		}
	}
	return units
}
