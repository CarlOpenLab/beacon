// Package web carries the built browser client.
//
// The assets come from `pnpm --dir web run build`, which writes into dist/ and
// is embedded here so one binary serves both the PC shell and a phone on the
// same LAN. Nothing in this package talks to a PTY.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
)

// The `all:` prefix keeps dotfiles, so the embed pattern still matches in a
// clean checkout where dist/ holds nothing but the .gitkeep marker.
//
//go:embed all:dist
var assets embed.FS

const missingPage = `前端尚未构建。

    pnpm --dir web run build     # 写入 internal/web/dist
    go build ./cmd/tfd

开发时可用 tfd -web-dir internal/web/dist 直接读取磁盘产物，免去重新编译。
`

// FS returns the built client rooted at dist/. The boolean is false when the
// binary was built before the frontend existed; callers surface that instead of
// serving a blank page.
func FS() (fs.FS, bool) {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}

// Config tells a client which transport this server speaks. One bundle is
// served by two different roles — a daemon that owns PTYs and a relay that only
// routes — so the client asks instead of guessing from its own URL.
func Config(mode, ws string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, `{"mode":%q,"ws":%q}`, mode, ws)
	}
}

// Handler serves the client. A non-empty devDir reads from disk instead of the
// embedded copy, which is what the watch-based dev loop uses.
func Handler(devDir string) http.Handler {
	var fsys fs.FS
	if devDir != "" {
		fsys = os.DirFS(devDir)
	} else {
		sub, ok := FS()
		if !ok {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprint(w, missingPage)
			})
		}
		fsys = sub
	}

	files := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Vite hashes asset names, so those may be cached forever. index.html
		// must not be: a returning phone would otherwise keep an old client.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
