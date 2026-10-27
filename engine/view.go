package main

// 길게 눌러 미리보기 (1.1.0)
//   이미 옮긴 사진 · 동영상: 맥에 있는 파일을 그대로 보여 줘요 (다시 받지 않아요).
//   아직 iPhone 에만 있는 것: 원본을 Support/cache/view/<파일 id>.<확장자> 로 받아 두고 보여 줘요.
//     받다 끊기면 이어 받고, 한 번 받은 것은 다음에 바로 보여요. 이틀이 지나거나 3GB 를 넘으면 오래된 것부터 지워요.
//   HEIC · DNG 같은 사진은 맥의 sips 로 JPEG 로 바꿔서 보여 줘요 (화면 크기 정도로 줄여서).

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type viewJob struct {
	done  int64
	size  int64
	ready bool
	err   string
	wait  chan struct{}
}

type Viewer struct {
	dir  string
	dev  Device
	mu   sync.Mutex
	jobs map[string]*viewJob
	sem  chan struct{}
	conv sync.Mutex
}

func NewViewer(support string, dev Device) *Viewer {
	v := &Viewer{dir: filepath.Join(support, "cache", "view"), dev: dev, jobs: map[string]*viewJob{}, sem: make(chan struct{}, 2)}
	os.MkdirAll(v.dir, 0o755)
	go v.clean()
	return v
}

// clean: 이틀 넘은 것, 그리고 3GB 를 넘으면 오래된 것부터 지워요
func (v *Viewer) clean() {
	defer recoverLog("view clean")
	ents, _ := os.ReadDir(v.dir)
	type fi struct {
		p    string
		t    time.Time
		size int64
	}
	var all []fi
	var total int64
	for _, e := range ents {
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(v.dir, e.Name())
		if time.Since(info.ModTime()) > 48*time.Hour {
			os.Remove(p)
			continue
		}
		all = append(all, fi{p, info.ModTime(), info.Size()})
		total += info.Size()
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t.Before(all[j].t) })
	for _, f := range all {
		if total <= 3<<30 {
			break
		}
		os.Remove(f.p)
		total -= f.size
	}
}

// pick: 묶음에서 보여 줄 파일 (photo = 사진, motion = Live Photo 영상 또는 동영상)
func pick(a *Asset, part string) *FileRef {
	for _, f := range a.Files {
		if part == "motion" && f.Video {
			return f
		}
		if part != "motion" && !f.Video {
			return f
		}
	}
	if a.K == "v" && len(a.Files) > 0 {
		return a.Files[0]
	}
	return nil
}

func (v *Viewer) cachePath(f *FileRef) string {
	return filepath.Join(v.dir, f.ID+strings.ToLower(filepath.Ext(f.Name)))
}

// local: 맥에 이미 있는 파일 (옮겨 둔 것 또는 미리보기용으로 받아 둔 것)
func (v *Viewer) local(f *FileRef) string {
	if r, ok := app.journal.Get(f.ID); ok {
		if st, err := os.Stat(r.Dest); err == nil && st.Size() == f.Size {
			return r.Dest
		}
	}
	p := v.cachePath(f)
	if st, err := os.Stat(p); err == nil && st.Size() == f.Size {
		now := time.Now()
		os.Chtimes(p, now, now)
		return p
	}
	return ""
}

// start: 아직 맥에 없으면 받기 시작해요 (이미 받는 중이면 그 작업)
func (v *Viewer) start(f *FileRef) *viewJob {
	v.mu.Lock()
	defer v.mu.Unlock()
	if j, ok := v.jobs[f.ID]; ok && j.err == "" {
		return j
	}
	j := &viewJob{size: f.Size, wait: make(chan struct{})}
	v.jobs[f.ID] = j
	go func() {
		defer recoverLog("view fetch")
		defer close(j.wait)
		v.sem <- struct{}{}
		defer func() { <-v.sem }()
		dst := v.cachePath(f)
		part := dst + ".part"
		var off int64
		if st, err := os.Stat(part); err == nil && st.Size() <= f.Size {
			off = st.Size()
		}
		var err error
		for try := 0; try < 3; try++ {
			if st := v.dev.State(); st != "ready" {
				err = errors.New(notReadyMsg(st))
				break
			}
			_, err = v.dev.Fetch(context.Background(), f.H, part, off, f.Size, func(n int64) {
				v.mu.Lock()
				j.done = n
				v.mu.Unlock()
			})
			if err == nil {
				break
			}
			if st, e := os.Stat(part); e == nil {
				off = st.Size()
			}
			time.Sleep(500 * time.Millisecond)
		}
		if err == nil {
			if st, e := os.Stat(part); e != nil || st.Size() != f.Size {
				err = errors.New("다 받지 못했어요")
			} else {
				err = os.Rename(part, dst)
			}
		}
		v.mu.Lock()
		if err != nil {
			j.err = err.Error()
			log.Printf("미리보기 받기 실패 %s: %v", f.Name, err)
		} else {
			j.ready, j.done = true, f.Size
		}
		v.mu.Unlock()
	}()
	return j
}

func (v *Viewer) resolve(r *http.Request) (*FileRef, error) {
	a, ok := app.lib.Asset(r.URL.Query().Get("id"))
	if !ok {
		return nil, errors.New("사진을 찾을 수 없어요")
	}
	f := pick(a, r.URL.Query().Get("p"))
	if f == nil {
		return nil, errors.New("보여 줄 파일이 없어요")
	}
	return f, nil
}

// Status: {ready, done, size, local} — 동영상은 이걸 보며 진행률을 보여 주다가 다 받으면 재생해요
func (v *Viewer) Status(w http.ResponseWriter, r *http.Request) {
	f, err := v.resolve(r)
	if err != nil {
		jsonError(w, 404, err.Error())
		return
	}
	if v.local(f) != "" {
		writeJSON(w, map[string]any{"ready": true, "done": f.Size, "size": f.Size})
		return
	}
	j := v.start(f)
	v.mu.Lock()
	defer v.mu.Unlock()
	writeJSON(w, map[string]any{"ready": j.ready, "done": j.done, "size": j.size, "error": j.err})
}

// Serve: 파일 내용 (사진은 필요하면 JPEG 로 바꿔서). 아직 없으면 받을 때까지 기다려요.
func (v *Viewer) Serve(w http.ResponseWriter, r *http.Request) {
	f, err := v.resolve(r)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	p := v.local(f)
	if p == "" {
		j := v.start(f)
		select {
		case <-j.wait:
		case <-r.Context().Done():
			return
		case <-time.After(3 * time.Minute):
		}
		if p = v.local(f); p == "" {
			http.Error(w, "iPhone에서 받지 못했어요", 503)
			return
		}
	}
	if !f.Video {
		p = v.displayable(f, p)
	}
	w.Header().Set("Cache-Control", "private, max-age=600")
	if strings.EqualFold(filepath.Ext(p), ".mov") {
		w.Header().Set("Content-Type", "video/quicktime")
	}
	http.ServeFile(w, r, p)
}

var needsConvert = map[string]bool{".heic": true, ".heif": true, ".dng": true, ".tif": true, ".tiff": true, ".raw": true, ".avif": true}

// displayable: 화면이 바로 못 그리는 사진(HEIC 등)은 JPEG 로 바꿔요 (맥의 sips)
func (v *Viewer) displayable(f *FileRef, src string) string {
	if !needsConvert[strings.ToLower(filepath.Ext(src))] || runtime.GOOS != "darwin" {
		return src
	}
	out := filepath.Join(v.dir, f.ID+".view.jpg")
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		now := time.Now()
		os.Chtimes(out, now, now)
		return out
	}
	v.conv.Lock()
	defer v.conv.Unlock()
	if fileExists(out) {
		return out
	}
	tmp := out + ".tmp.jpg"
	cmd := exec.Command("/usr/bin/sips", "-s", "format", "jpeg", "-s", "formatOptions", "88", "-Z", "2560", src, "--out", tmp)
	if b, err := cmd.CombinedOutput(); err != nil {
		log.Printf("미리보기 변환 실패 %s: %v %s", f.Name, err, lastLine(string(b)))
		os.Remove(tmp)
		return src
	}
	os.Rename(tmp, out)
	return out
}
