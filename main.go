// hello-system: a Helix spoke. Implements the minimum contract:
//
//   GET  /healthz                  liveness
//   POST /__helix/bootstrap        platform calls this to seed an admin
//   POST /invoke/echo              an example business endpoint
//
// The Gateway proxies requests here under /invoke/hello-system/{rest},
// so the path the spoke sees is /invoke/{rest}. The spoke trusts the
// gateway-set X-Helix-* headers (the gateway is the only thing on the
// private network that can reach the spoke), so we do not re-validate
// tokens here.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// In a real spoke this is the spoke's own users table. For the
// reference impl we keep a thin in-memory record so the bootstrap
// contract can be exercised end-to-end.
type adminRecord struct {
	Email     string
	Name      string
	LoginCode string
	CreatedAt time.Time
}

var (
	adminMu sync.Mutex
	admin   *adminRecord
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("POST /__helix/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		// The platform sends a shared token in the header so the spoke
		// can confirm the request originates from Helix's
		// deploy-controller and not from a random user. The token is
		// generated at deploy time and injected as HELIX_BOOTSTRAP_TOKEN.
		expected := os.Getenv("HELIX_BOOTSTRAP_TOKEN")
		if expected == "" || subtle.ConstantTimeCompare(
			[]byte(r.Header.Get("X-Helix-Bootstrap-Token")), []byte(expected),
		) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var body struct {
			AdminEmail string `json:"admin_email"`
			AdminName  string `json:"admin_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AdminEmail == "" {
			http.Error(w, "bad request: admin_email required", http.StatusBadRequest)
			return
		}

		adminMu.Lock()
		defer adminMu.Unlock()
		if admin != nil {
			// Already bootstrapped. Return the existing login URL so
			// the operator can sign in even if the wizard re-ran.
			respondBootstrap(w, admin)
			return
		}
		admin = &adminRecord{
			Email:     body.AdminEmail,
			Name:      body.AdminName,
			LoginCode: time.Now().UTC().Format("20060102") + "-hellosystem",
			CreatedAt: time.Now(),
		}
		respondBootstrap(w, admin)
	})

	mux.HandleFunc("POST /invoke/echo", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		caller := r.Header.Get("X-Helix-Caller-System")
		user := r.Header.Get("X-Helix-User-Email")
		reqID := r.Header.Get("X-Helix-Request-ID")
		logger.Info("echo invoked",
			"caller", caller, "user", user, "request_id", reqID, "size", len(body))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"caller":      caller,
			"user":        user,
			"request_id":  reqID,
			"received_at": time.Now().UTC().Format(time.RFC3339Nano),
			"echo":        string(body),
		})
	})

	// A trivial admin landing page, served when the operator hits the
	// login_url returned by bootstrap. Real spokes will mint a session
	// here; we just render a confirmation.
	mux.HandleFunc("GET /admin/{code}", func(w http.ResponseWriter, r *http.Request) {
		adminMu.Lock()
		a := admin
		adminMu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if a == nil || r.PathValue("code") != a.LoginCode {
			http.Error(w, "invalid or expired login link", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>hello-system</title>
<body style="font-family:ui-sans-serif,system-ui;background:#f3efe7;color:#101418;padding:48px">
<h1 style="font-family:Charter,Georgia,serif;font-weight:500">Signed in to hello-system</h1>
<p>%s &lt;%s&gt; bootstrapped at %s.</p>
<p style="color:#5b5a55">This is the reference spoke's admin landing. Real spokes mint a real session here.</p>
</body>`, htmlEscape(a.Name), htmlEscape(a.Email), a.CreatedAt.UTC().Format(time.RFC3339))
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "hello-system: a Helix spoke. Try POST /invoke/echo via the gateway.")
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("hello-system listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server", "err", err)
		os.Exit(1)
	}
}

func respondBootstrap(w http.ResponseWriter, a *adminRecord) {
	w.Header().Set("Content-Type", "application/json")
	host := os.Getenv("HELIX_SYSTEM_HOST") // e.g. hello-system.helixisflying.duckdns.org
	scheme := "https://"
	if host == "" {
		host = "hello-system.local"
		scheme = "http://"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"login_url": scheme + host + "/admin/" + a.LoginCode,
		"message":   "Admin bootstrapped. Open the login_url to land in this spoke as its first user.",
		"admin": map[string]any{
			"email": a.Email,
			"name":  a.Name,
		},
	})
}

func htmlEscape(s string) string {
	r := []byte(s)
	out := make([]byte, 0, len(r))
	for _, c := range r {
		switch c {
		case '<':
			out = append(out, []byte("&lt;")...)
		case '>':
			out = append(out, []byte("&gt;")...)
		case '&':
			out = append(out, []byte("&amp;")...)
		case '"':
			out = append(out, []byte("&quot;")...)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// touch 20260512-214612
