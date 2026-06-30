package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kobylinski/yucca/internal/store"
)

// DaemonInfo is written to ~/.yucca/daemon.json on startup for client discovery.
type DaemonInfo struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Addr      string    `json:"addr"`
	StartedAt time.Time `json:"started_at"`
}

// LockfilePath returns the path to the daemon lockfile.
func LockfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".yucca", "daemon.json")
}

// WriteLockfile writes daemon info to the lockfile.
func WriteLockfile(port int) error {
	info := DaemonInfo{
		PID:       os.Getpid(),
		Port:      port,
		Addr:      fmt.Sprintf("http://127.0.0.1:%d", port),
		StartedAt: time.Now(),
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(LockfilePath(), data, 0600)
}

// RemoveLockfile removes the daemon lockfile, but only if it still points at
// this process. During a managed restart launchd can overlap the outgoing and
// incoming daemon: the new one may have already written its own lockfile, and
// the outgoing daemon must not delete that. Without this check the daemon ends
// up running with no lockfile, so clients (the tray app) can't discover it.
func RemoveLockfile() {
	data, err := os.ReadFile(LockfilePath())
	if err != nil {
		return
	}
	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err == nil && info.PID != os.Getpid() {
		return
	}
	os.Remove(LockfilePath())
}

// ReadLockfile reads the daemon lockfile and returns the info.
// Returns nil if the file doesn't exist or the daemon process is dead.
func ReadLockfile() *DaemonInfo {
	data, err := os.ReadFile(LockfilePath())
	if err != nil {
		return nil
	}
	var info DaemonInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	// Check if the process is still alive
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return nil
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is dead — stale lockfile
		os.Remove(LockfilePath())
		return nil
	}
	return &info
}

// DiscoverAddr returns the daemon address using priority:
// explicit flag > lockfile > YUCCA_DAEMON env > default.
func DiscoverAddr(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if info := ReadLockfile(); info != nil {
		return info.Addr
	}
	if env := os.Getenv("YUCCA_DAEMON"); env != "" {
		return env
	}
	return "http://127.0.0.1:9777"
}

type Daemon struct {
	Store       *store.Store
	Queue       *RequestQueue
	WS          *WSHub
	Sessions    *SessionTracker
	Approvals   *SessionApprovals
	Port        int
	IdleTimeout time.Duration
	listener    net.Listener
	server      *http.Server
	ready       chan struct{}
	cancel      context.CancelFunc
}

type Config struct {
	StoreDir    string
	Port        int
	IdleTimeout time.Duration // shutdown after no active sessions; 0 = disabled
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		StoreDir:    filepath.Join(home, ".yucca"),
		Port:        0,
		IdleTimeout: 5 * time.Minute,
	}
}

func New(cfg Config) (*Daemon, error) {
	s, err := store.New(cfg.StoreDir)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	ws := NewWSHub()
	approvals := NewSessionApprovals()
	d := &Daemon{
		Store:       s,
		Queue:       NewRequestQueue(),
		WS:          ws,
		Approvals:   approvals,
		Port:        cfg.Port,
		IdleTimeout: cfg.IdleTimeout,
		ready:       make(chan struct{}),
	}
	// When an agent session ends (reaped after no heartbeats), forget its
	// per-session approvals so secrets are re-confirmed next session.
	d.Sessions = NewSessionTracker(60*time.Second, func(reaped []string) {
		for _, slug := range reaped {
			approvals.ClearProject(slug)
		}
		ws.Broadcast(WSEvent{Type: "sessions_changed"})
	})
	return d, nil
}

// localhostGuard enforces that the daemon only serves genuinely local traffic.
// The daemon already binds 127.0.0.1, but a browser can still be steered at it
// (DNS rebinding, or a plain cross-site POST) from a website the user visits.
// We reject any request whose Host is not the loopback listen address, and any
// request carrying a non-loopback Origin, and we forbid framing. This makes
// "serve localhost only" a real boundary without introducing an auth system.
// Legitimate clients (the Go MCP server, the Swift app, and the UI served from
// 127.0.0.1) all send a loopback Host and either no Origin or a loopback one.
func localhostGuard(next http.Handler, port int) http.Handler {
	hosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
	origins := map[string]bool{
		fmt.Sprintf("http://127.0.0.1:%d", port): true,
		fmt.Sprintf("http://localhost:%d", port): true,
		fmt.Sprintf("http://[::1]:%d", port):     true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "forbidden: non-local Host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !origins[o] {
			http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (d *Daemon) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	d.registerAPI(mux)

	uiFS, err := fs.Sub(uiDist, "ui_dist")
	if err != nil {
		return fmt.Errorf("embed ui: %w", err)
	}
	fileServer := http.FileServer(http.FS(uiFS))
	mux.Handle("/", spaHandler(uiFS, fileServer))

	addr := fmt.Sprintf("127.0.0.1:%d", d.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	d.listener = ln
	d.Port = ln.Addr().(*net.TCPAddr).Port

	d.server = &http.Server{Handler: localhostGuard(mux, d.Port)}

	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	log.Printf("yucca daemon listening on http://127.0.0.1:%d", d.Port)
	if err := WriteLockfile(d.Port); err != nil {
		log.Printf("warning: could not write lockfile: %v", err)
	}
	close(d.ready)

	go func() {
		<-ctx.Done()
		RemoveLockfile()
		_ = d.server.Shutdown(context.Background())
	}()

	if d.IdleTimeout > 0 {
		go d.idleMonitor(ctx)
	}

	if err := d.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (d *Daemon) Ready() <-chan struct{} {
	return d.ready
}

func (d *Daemon) Addr() string {
	<-d.ready
	return fmt.Sprintf("http://127.0.0.1:%d", d.Port)
}

// Handler returns the daemon's bare API handler — the same routes Start serves,
// without the localhost guard or the embedded UI fileserver. Intended for
// in-process tests that drive the API directly via httptest.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	d.registerAPI(mux)
	return mux
}

// idleMonitor shuts the daemon down when no sessions are active for IdleTimeout.
func (d *Daemon) idleMonitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	idleSince := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(d.Sessions.Active()) > 0 {
				idleSince = time.Now()
			} else if time.Since(idleSince) >= d.IdleTimeout {
				log.Printf("no active sessions for %s, shutting down", d.IdleTimeout)
				d.cancel()
				return
			}
		}
	}
}

// spaHandler serves static files if they exist, otherwise falls back to index.html.
func spaHandler(uiFS fs.FS, fileServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path != "/" {
			// Strip leading slash for fs.Stat
			fsPath := path[1:]
			if _, err := fs.Stat(uiFS, fsPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Fall back to index.html (SPA routing)
		indexData, err := fs.ReadFile(uiFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexData)
	})
}
