// Command server-health is a small dashboard for one machine: live CPU,
// memory, load and disk readings, plus the state of the systemd units you
// name, which it can start, stop and restart.
//
// It ships as a single binary with the frontend embedded, binds to loopback
// and expects to be reached over a private network such as Tailscale.
package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/andrinoff/server-health/internal/api"
	"github.com/andrinoff/server-health/internal/host"
	"github.com/andrinoff/server-health/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	services := flag.String("services", "server-health",
		"comma-separated systemd units to show and control (add the ones you care about)")
	disk := flag.String("disk", "/", "filesystem to report disk usage for")
	demoMode := flag.Bool("demo", false, "serve invented readings and units so the page can be previewed off Linux")
	flag.Parse()

	static, err := web.FS()
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	sampler := host.NewSampler(host.Options{Path: *disk})
	stop := make(chan struct{})
	defer close(stop)
	go sampler.Start(stop)

	if *demoMode {
		log.Printf("DEMO MODE: readings and units are invented; nothing on this machine is read or changed")
	}

	srv := &http.Server{
		Addr: *addr,
		Handler: api.NewServer(static, api.Config{
			Host:     sampler,
			Services: strings.Split(*services, ","),
			Demo:     *demoMode,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("server-health listening on %s (services: %s, disk: %s)", *addr, *services, *disk)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
