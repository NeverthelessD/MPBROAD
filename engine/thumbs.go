package main

// 미리보기 그림 (Support/cache/thumbs/<파일 id>.jpg). iPhone 에서 작은 그림만 받아 와요.

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Thumbs struct {
	dir    string
	dev    Device
	sem    chan struct{}
	mu     sync.Mutex
	failed map[string]time.Time
	busy   map[string]chan struct{}
}

func NewThumbs(support string, dev Device) *Thumbs {
	t := &Thumbs{dir: filepath.Join(support, "cache", "thumbs"), dev: dev, sem: make(chan struct{}, 4),
		failed: map[string]time.Time{}, busy: map[string]chan struct{}{}}
	os.MkdirAll(t.dir, 0o755)
	go t.clean()
	return t
}

// clean: 60일 넘게 안 본 미리보기는 지워요
func (t *Thumbs) clean() {
	defer recoverLog("thumb clean")
	time.Sleep(30 * time.Second)
	ents, _ := os.ReadDir(t.dir)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 60*24*time.Hour {
			os.Remove(filepath.Join(t.dir, e.Name()))
		}
	}
}

func (t *Thumbs) Serve(w http.ResponseWriter, r *http.Request, id string) {
	if len(id) != 16 {
		http.NotFound(w, r)
		return
	}
	p := filepath.Join(t.dir, id+".jpg")
	if fileExists(p) {
		t.send(w, r, p)
		return
	}
	f, ok := app.lib.File(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t.mu.Lock()
	if at, bad := t.failed[id]; bad && time.Since(at) < time.Minute {
		t.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	wait, running := t.busy[id]
	if !running {
		wait = make(chan struct{})
		t.busy[id] = wait
	}
	t.mu.Unlock()
	if running {
		select {
		case <-wait:
		case <-r.Context().Done():
			return
		}
	} else {
		func() {
			defer func() {
				t.mu.Lock()
				delete(t.busy, id)
				t.mu.Unlock()
				close(wait)
			}()
			select {
			case t.sem <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			defer func() { <-t.sem }()
			// 가져오는 중에는 미리보기를 조금 천천히 (가져오기가 먼저)
			if app.jobs != nil && app.jobs.Running() {
				time.Sleep(150 * time.Millisecond)
			}
			if err := t.dev.Thumb(f.H, p, 320); err != nil {
				os.Remove(p)
				t.mu.Lock()
				t.failed[id] = time.Now()
				t.mu.Unlock()
			}
		}()
	}
	if fileExists(p) {
		t.send(w, r, p)
		return
	}
	http.NotFound(w, r)
}

func (t *Thumbs) send(w http.ResponseWriter, r *http.Request, p string) {
	now := time.Now()
	os.Chtimes(p, now, now)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}
