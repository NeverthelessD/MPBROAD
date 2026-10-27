// MyPhotoBook_ROAD — iPhone 사진 · 동영상을 맥으로 옮기고 연 / 월 / 일로 정리해요 (macOS, Apple Silicon)
// made by. Nevertheless_D
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// build is the app version, set with -ldflags "-X main.build=1.2.3".
var build = "0.0.0-dev"

//go:embed web
var webFiles embed.FS

const appName = "MyPhotoBook_ROAD"

type Settings struct {
	Appearance       string `json:"appearance"` // system | light | dark
	AutoUpdate       bool   `json:"autoUpdate"`
	DestMode         string `json:"destMode"`   // default | custom
	CustomDest       string `json:"customDest"` // 다른 폴더
	SkipDone         bool   `json:"skipDone"`   // 이미 옮긴 항목 건너뛰기
	KeepLive         bool   `json:"keepLive"`   // Live Photo 사진과 영상을 함께
	AskDelete        bool   `json:"askDelete"`  // 복사 확인 후 iPhone에서 지울지 묻기
	ResumeJobs       bool   `json:"resumeJobs"` // 멈추면 다음에 이어서 하기
	AutoResume       bool   `json:"autoResume"` // 잠금을 풀면 자동으로 이어서 하기
	PreviewDefault   bool   `json:"previewDefault"`
	DelAfter         bool   `json:"delAfter"` // 아래 막대의 '확인 후 iPhone에서 지우기' 상태
	DelAfterTouched  bool   `json:"delAfterTouched"`
	LastDestReminder string `json:"-"`
}

type App struct {
	support string
	token   string
	port    int

	mu       sync.Mutex
	settings Settings

	dev     Device
	lib     *Library
	journal *Journal
	jobs    *Jobs
	thumbs  *Thumbs
	view    *Viewer
	upd     *Updater

	lastSeen atomic.Int64
}

var app *App

// started 는 단조 시계 기준점이에요 (잠자는 동안 늘어나지 않아요).
var started = time.Now()

func touch() { app.lastSeen.Store(int64(time.Since(started))) }

func env(k string) string { return os.Getenv("MPB_" + k) }

func defaultSupport() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", appName)
	}
	return filepath.Join(home, ".myphotobook-road")
}

func main() {
	log.SetFlags(log.LstdFlags)
	support := env("SUPPORT")
	if support == "" {
		support = defaultSupport()
	}
	os.MkdirAll(support, 0o755)
	if env("SUPERVISOR") == "1" {
		runSupervisor(support)
		return
	}
	token := env("TOKEN")
	if token == "" {
		token = randID(16)
	}
	app = &App{support: support, token: token}
	os.Rename(filepath.Join(support, "engine.log"), filepath.Join(support, "engine.prev.log"))
	if f, err := os.OpenFile(filepath.Join(support, "engine.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, f))
	}
	app.loadSettings()
	touch()
	app.journal = OpenJournal(support)
	app.dev = newDevice(support)
	app.lib = NewLibrary(app.dev, app.journal)
	app.thumbs = NewThumbs(support, app.dev)
	app.view = NewViewer(support, app.dev)
	app.jobs = NewJobs(support, app.dev, app.lib, app.journal)
	app.upd = NewUpdater(support)
	app.dev.Start(func() { app.lib.Changed() })

	go heartbeat(support)
	ln := listen(env("PORT"), env("PORT_WAIT") == "1")
	app.port = ln.Addr().(*net.TCPAddr).Port
	writeFileAtomic(filepath.Join(support, "token"), []byte(token), 0o600)
	writeFileAtomic(filepath.Join(support, "port"), []byte(strconv.Itoa(app.port)), 0o644)
	writeFileAtomic(filepath.Join(support, "lastport"), []byte(strconv.Itoa(app.port)), 0o644)
	log.Printf("%s %s listening on %d (pid %d, 지킴이 %v, 기기 %s)", appName, build, app.port, os.Getpid(), env("CHILD") == "1", app.dev.Kind())

	if app.port != defaultPort {
		go func() {
			defer recoverLog("diag listener")
			if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultPort)); err == nil {
				m := http.NewServeMux()
				m.HandleFunc("/diag", app.handleDiag)
				http.Serve(ln, hostGuard(m))
			}
		}()
	}
	go app.watchParent()
	go func() { defer recoverLog("autoLoop"); app.upd.autoLoop() }()
	srv := &http.Server{Handler: hostGuard(recordRequests(app.routes())), ErrorLog: log.Default()}
	log.Fatal(srv.Serve(ln))
}

// defaultPort: 늘 같은 주소를 쓰면 엔진이 다시 켜져도 열려 있던 창이 이어지고, 진단 페이지도 찾기 쉬워요.
const defaultPort = 47931

func listen(pref string, wait bool) net.Listener {
	p, err := strconv.Atoi(pref)
	if err != nil || p <= 0 {
		p = defaultPort
	}
	tries := 1
	if wait {
		tries = 20
	}
	for i := 0; i < tries; i++ {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			return ln
		}
		time.Sleep(150 * time.Millisecond)
	}
	if p != defaultPort {
		if ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultPort)); err == nil {
			return ln
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	return ln
}

// ---------------------------------------------------------------- lifecycle

// nativeShell: Dock 에 보이는 맥 앱(app/shell/main.swift)이 엔진을 켰는지
func nativeShell() bool { return env("SHELL") == "native" }

// watchParent: 지킴이(부모)가 사라지면 엔진도 끝나요 — 혼자 남아 몰래 돌지 않게
func (a *App) watchParent() {
	if env("CHILD") != "1" {
		return
	}
	ppid := os.Getppid()
	for range time.Tick(2 * time.Second) {
		if os.Getppid() != ppid {
			a.shutdownWhy("지킴이가 사라짐")
		}
	}
}

var shuttingDown atomic.Bool

func (a *App) shutdownWhy(why string) {
	if !shuttingDown.CompareAndSwap(false, true) {
		return
	}
	log.Printf("shutdown: %s", why)
	if a.jobs != nil {
		a.jobs.Shutdown("앱이 꺼졌어요")
	}
	if a.dev != nil {
		a.dev.Stop()
	}
	markCleanExit(a.support, why)
	if b, err := os.ReadFile(filepath.Join(a.support, "port")); err == nil && string(b) == strconv.Itoa(a.port) {
		os.Remove(filepath.Join(a.support, "port"))
	}
	os.Exit(0)
}

// ---------------------------------------------------------------- settings

func (a *App) settingsPath() string { return filepath.Join(a.support, "settings.json") }

func (a *App) loadSettings() {
	a.settings = Settings{Appearance: "system", AutoUpdate: true, DestMode: "default", SkipDone: true, KeepLive: true,
		AskDelete: true, ResumeJobs: true, AutoResume: true, PreviewDefault: true, DelAfter: true}
	if b, err := os.ReadFile(a.settingsPath()); err == nil {
		_ = json.Unmarshal(b, &a.settings)
	}
	if a.settings.DestMode != "custom" {
		a.settings.DestMode = "default"
	}
}

func (a *App) saveSettings() {
	b, _ := json.MarshalIndent(a.settings, "", "  ")
	writeFileAtomic(a.settingsPath(), b, 0o644)
}

func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func defaultDest() string {
	if d := env("DEFAULT_DEST"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Pictures", "MyPhotoBook")
}

// Dest: 사진을 넣을 폴더 (다른 폴더를 골랐는데 없어졌으면 기본 폴더)
func (a *App) Dest() string {
	s := a.Settings()
	if s.DestMode == "custom" && s.CustomDest != "" {
		return s.CustomDest
	}
	return defaultDest()
}

func (a *App) destInfo() map[string]any {
	s := a.Settings()
	d := a.Dest()
	return map[string]any{
		"path":      d,
		"label":     destLabel(d),
		"mode":      s.DestMode,
		"custom":    s.CustomDest,
		"customOK":  s.CustomDest != "" && dirExists(s.CustomDest),
		"default":   defaultDest(),
		"defLabel":  destLabel(defaultDest()),
		"available": volumeOK(d),
	}
}

// destLabel: 화면에 보여 줄 짧은 이름 (예: 사진 › MyPhotoBook)
func destLabel(p string) string {
	home, _ := os.UserHomeDir()
	names := map[string]string{"Pictures": "사진", "Desktop": "데스크탑", "Documents": "문서", "Downloads": "다운로드", "Movies": "동영상"}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		parts := strings.Split(rel, string(filepath.Separator))
		if n, ok := names[parts[0]]; ok {
			parts[0] = n
		}
		if len(parts) > 3 {
			parts = append([]string{parts[0], "…"}, parts[len(parts)-2:]...)
		}
		return strings.Join(parts, " › ")
	}
	if strings.HasPrefix(p, "/Volumes/") {
		parts := strings.Split(strings.TrimPrefix(p, "/Volumes/"), "/")
		if len(parts) > 3 {
			parts = append([]string{parts[0], "…"}, parts[len(parts)-2:]...)
		}
		return strings.Join(parts, " › ")
	}
	return p
}

// volumeOK: 외장 디스크를 골랐는데 빠져 있으면 false
func volumeOK(p string) bool {
	if strings.HasPrefix(p, "/Volumes/") {
		parts := strings.SplitN(strings.TrimPrefix(p, "/Volumes/"), "/", 2)
		return dirExists("/Volumes/" + parts[0])
	}
	return true
}

func (a *App) settingsJSON() map[string]any {
	s := a.Settings()
	home, _ := os.UserHomeDir()
	return map[string]any{
		"appearance":     s.Appearance,
		"autoUpdate":     s.AutoUpdate,
		"skipDone":       s.SkipDone,
		"keepLive":       s.KeepLive,
		"askDelete":      s.AskDelete,
		"resumeJobs":     s.ResumeJobs,
		"autoResume":     s.AutoResume,
		"previewDefault": s.PreviewDefault,
		"delAfter":       s.DelAfter,
		"dest":           a.destInfo(),
		"home":           home,
		"build":          build,
		"platform":       runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// ---------------------------------------------------------------- routes

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.URL.Query().Get("t") != a.token {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, "<meta charset=utf-8><body style='font:15px -apple-system;padding:40px'>MyPhotoBook_ROAD 앱을 다시 실행해 주세요.</body>")
				return
			}
			b, _ := webFiles.ReadFile("web/index.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "no-cache")
		} else if strings.HasPrefix(r.URL.Path, "/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=604800")
		}
		static.ServeHTTP(w, r)
	})

	mux.HandleFunc("/diag", a.handleDiag)

	api := func(path string, h http.HandlerFunc) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			t := r.Header.Get("x-token")
			if t == "" {
				t = r.URL.Query().Get("t")
			}
			if t != a.token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			touch()
			defer func() {
				if v := recover(); v != nil {
					log.Printf("PANIC in %s: %v\n%s", path, v, debug.Stack())
					jsonError(w, 500, "앱 안에서 문제가 생겼어요. 다시 시도해 주세요")
				}
			}()
			h(w, r)
		})
	}

	api("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "build": build, "shell": env("SHELL")})
	})
	api("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); a.shutdownWhy("종료 버튼") }()
	})
	api("/api/clientlog", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Items []string `json:"items"`
		}
		readJSON(r, &in)
		for i, x := range in.Items {
			if i >= 30 {
				break
			}
			if len(x) > 600 {
				x = x[:600]
			}
			diag.clientErr(x)
			log.Printf("client: %s", x)
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/poll", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"build":  build,
			"shell":  env("SHELL"),
			"rev":    a.lib.Rev(),
			"device": a.dev.Status(),
			"job":    a.jobs.State(),
			"update": a.upd.State(),
			"dest":   a.destInfo(),
		})
	})
	api("/api/items", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, a.lib.Snapshot(a.Settings()))
	})
	api("/api/thumb", func(w http.ResponseWriter, r *http.Request) {
		a.thumbs.Serve(w, r, r.URL.Query().Get("id"))
	})
	api("/api/view", func(w http.ResponseWriter, r *http.Request) { a.view.Serve(w, r) })
	api("/api/view/status", func(w http.ResponseWriter, r *http.Request) { a.view.Status(w, r) })
	api("/api/settings", a.handleSettings)
	api("/api/choose-folder", a.handleChooseFolder)
	api("/api/reveal", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Path string `json:"path"`
		}
		readJSON(r, &in)
		p := in.Path
		if p == "" {
			p = a.Dest()
			os.MkdirAll(p, 0o755)
		}
		if fileExists(p) {
			if dirExists(p) {
				openPaths([]string{p})
			} else {
				revealPaths([]string{p})
			}
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	// 가져오기 · 지우기
	api("/api/import", a.handleImport)
	api("/api/delete/plan", a.handleDeletePlan)
	api("/api/delete", a.handleDelete)
	api("/api/job/pause", func(w http.ResponseWriter, r *http.Request) { a.jobs.Pause(); writeJSON(w, a.jobs.State()) })
	api("/api/job/resume", func(w http.ResponseWriter, r *http.Request) {
		if err := a.jobs.Resume(); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		writeJSON(w, a.jobs.State())
	})
	api("/api/job/cancel", func(w http.ResponseWriter, r *http.Request) { a.jobs.Cancel(); writeJSON(w, a.jobs.State()) })
	api("/api/job/discard", func(w http.ResponseWriter, r *http.Request) { a.jobs.Discard(); writeJSON(w, a.jobs.State()) })
	api("/api/job/dismiss", func(w http.ResponseWriter, r *http.Request) { a.jobs.Dismiss(); writeJSON(w, a.jobs.State()) })
	api("/api/job/delete", func(w http.ResponseWriter, r *http.Request) {
		if err := a.jobs.DeleteVerified(); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		writeJSON(w, a.jobs.State())
	})

	api("/api/update/check", func(w http.ResponseWriter, r *http.Request) {
		a.upd.Check(true)
		writeJSON(w, a.upd.State())
	})
	api("/api/update/install", func(w http.ResponseWriter, r *http.Request) {
		go func() { defer recoverLog("install"); a.upd.Install() }()
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/rollback", func(w http.ResponseWriter, r *http.Request) {
		if err := a.upd.Rollback(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	api("/api/update/restart", func(w http.ResponseWriter, r *http.Request) {
		if a.jobs.Busy() {
			jsonError(w, 400, "가져오는 중이에요. 끝난 뒤에 다시 시작해 주세요")
			return
		}
		if err := a.upd.Restart(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	return mux
}

// Only answer requests addressed to 127.0.0.1 / localhost (DNS rebinding protection).
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var in map[string]json.RawMessage
		if err := readJSON(r, &in); err != nil {
			jsonError(w, 400, "bad request")
			return
		}
		a.mu.Lock()
		s := &a.settings
		for k, v := range in {
			switch k {
			case "appearance":
				var x string
				if json.Unmarshal(v, &x) == nil && (x == "system" || x == "light" || x == "dark") {
					s.Appearance = x
				}
			case "autoUpdate":
				json.Unmarshal(v, &s.AutoUpdate)
			case "skipDone":
				json.Unmarshal(v, &s.SkipDone)
			case "keepLive":
				json.Unmarshal(v, &s.KeepLive)
			case "askDelete":
				json.Unmarshal(v, &s.AskDelete)
				if !s.DelAfterTouched {
					s.DelAfter = s.AskDelete
				}
			case "resumeJobs":
				json.Unmarshal(v, &s.ResumeJobs)
			case "autoResume":
				json.Unmarshal(v, &s.AutoResume)
			case "previewDefault":
				json.Unmarshal(v, &s.PreviewDefault)
			case "delAfter":
				json.Unmarshal(v, &s.DelAfter)
				s.DelAfterTouched = true
			case "destMode":
				var x string
				if json.Unmarshal(v, &x) == nil && (x == "default" || x == "custom") {
					if x == "custom" && s.CustomDest == "" {
						break
					}
					s.DestMode = x
				}
			case "customDest":
				var x string
				if json.Unmarshal(v, &x) == nil {
					if x != "" && !dirExists(x) {
						a.mu.Unlock()
						jsonError(w, 400, "폴더를 찾을 수 없어요")
						return
					}
					s.CustomDest = x
					if x != "" {
						s.DestMode = "custom"
					} else {
						s.DestMode = "default"
					}
				}
			}
		}
		a.saveSettings()
		a.mu.Unlock()
		a.lib.Changed()
	}
	writeJSON(w, a.settingsJSON())
}

// ---------------------------------------------------------------- utils

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v)
}

func randID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeFileAtomic(path string, b []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func dirExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func init() {
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".mov", "video/quicktime")
	mime.AddExtensionType(".heic", "image/heic")
}

func recoverLog(where string) {
	if v := recover(); v != nil {
		log.Printf("PANIC in %s: %v\n%s", where, v, debug.Stack())
	}
}
