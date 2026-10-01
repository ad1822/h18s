// toggler serves a small page with on/off switches that scale allowlisted
// Deployments between 0 and 1 replica. Meant to be embedded in Homepage
// via its iframe widget.
//
// Config (env):
//
//	APPS         comma-separated Deployment names that may be toggled (required)
//	NAMESPACE    namespace of those Deployments (default "default")
//	LISTEN_ADDR  listen address (default ":8080")
//	KUBE_API     API base URL for local dev, e.g. http://127.0.0.1:8001 (kubectl proxy)
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var indexHTML []byte

type appStatus struct {
	Name    string `json:"name"`
	State   string `json:"state"` // running | starting | stopping | stopped | unknown
	Ready   int32  `json:"ready"`
	Desired int32  `json:"desired"`
	Error   string `json:"error,omitempty"`
}

type server struct {
	kube      *kubeClient
	namespace string
	apps      []string
	allowed   map[string]bool
}

func main() {
	apps := splitList(os.Getenv("APPS"))
	if len(apps) == 0 {
		log.Fatal("APPS is required, e.g. APPS=radarr,sonarr,jellyfin")
	}

	kube, err := newKubeClient()
	if err != nil {
		log.Fatal(err)
	}

	s := &server{
		kube:      kube,
		namespace: envOr("NAMESPACE", "default"),
		apps:      apps,
		allowed:   make(map[string]bool, len(apps)),
	}
	for _, a := range apps {
		s.allowed[a] = true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/apps", s.handleList)
	mux.HandleFunc("POST /api/apps/{name}/{action}", s.handleToggle)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{
		Addr:              envOr("LISTEN_ADDR", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s, namespace=%s, apps=%s", srv.Addr, s.namespace, strings.Join(apps, ","))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func (s *server) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	out := make([]appStatus, len(s.apps))
	var wg sync.WaitGroup
	for i, name := range s.apps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = s.status(r.Context(), name)
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

func (s *server) handleToggle(w http.ResponseWriter, r *http.Request) {
	// A custom header makes cross-origin requests need a CORS preflight,
	// which this server never approves, so other sites can't toggle apps.
	if r.Header.Get("X-Toggler") != "1" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing X-Toggler header"})
		return
	}

	name := r.PathValue("name")
	if !s.allowed[name] {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown app"})
		return
	}

	var replicas int32
	switch action := r.PathValue("action"); action {
	case "start":
		replicas = 1
	case "stop":
		replicas = 0
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "action must be start or stop"})
		return
	}

	if err := s.kube.scale(r.Context(), s.namespace, name, replicas); err != nil {
		log.Printf("scale %s to %d: %v", name, replicas, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("scaled %s to %d", name, replicas)
	writeJSON(w, http.StatusOK, s.status(r.Context(), name))
}

func (s *server) status(ctx context.Context, name string) appStatus {
	d, err := s.kube.getDeployment(ctx, s.namespace, name)
	if err != nil {
		return appStatus{Name: name, State: "unknown", Error: err.Error()}
	}

	desired := int32(1) // Kubernetes default when spec.replicas is unset
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	st := appStatus{Name: name, Ready: d.Status.ReadyReplicas, Desired: desired}

	switch {
	case desired == 0 && d.Status.Replicas > 0:
		st.State = "stopping"
	case desired == 0:
		st.State = "stopped"
	case d.Status.ReadyReplicas >= desired:
		st.State = "running"
	default:
		st.State = "starting"
	}
	return st
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
