package web

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed sw.js
var serviceWorkerSource string

// buildVersion names this build: a hash of the running binary, so it changes
// with every deploy that changes anything, and a restart keeps it. The service
// worker carries it, and its change is what tells open pages that a new
// version is out.
func buildVersion() string {
	if exe, err := os.Executable(); err == nil {
		if f, err := os.Open(exe); err == nil {
			defer f.Close()
			h := sha256.New()
			if _, err := io.Copy(h, f); err == nil {
				return hex.EncodeToString(h.Sum(nil)[:6])
			}
		}
	}
	// An unreadable binary: a new value per start (a restart then also reads
	// as a new version, which only costs a reload).
	return strconv.FormatInt(time.Now().Unix(), 36)
}

// serviceWorker serves /sw.js with the build in it. It must come from the
// root to control every page. no-store keeps the CDN from holding an old
// copy, which would hide a deploy until it expired.
func serviceWorker(version string) http.HandlerFunc {
	body := []byte(strings.ReplaceAll(serviceWorkerSource, "{{version}}", version))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	}
}
