package main

// iPhone 안의 파일들을 '사진 한 장' 단위(Asset)로 묶어요.
//   Live Photo = 같은 폴더 · 같은 이름의 사진(HEIC/JPG) + 영상(MOV)
//   RAW+JPEG  = 같은 이름의 사진 두 개 → 한 장
// 각 파일에는 늘 같은 id 를 붙여요 (폴더 · 이름 · 크기 · 찍은 날짜). 연결이 바뀌어도 같은 파일이면 같은 id.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FileRef struct {
	ID      string
	H       string
	Name    string
	Size    int64
	Created int64
	Folder  string
	Video   bool
	Dur     float64
}

type Asset struct {
	ID    string     `json:"id"`
	K     string     `json:"k"` // p 사진 · v 동영상 · l Live Photo
	N     string     `json:"n"`
	D     int64      `json:"d"` // 찍은 날짜 (유닉스 초)
	S     int64      `json:"s"` // 바이트 (묶음 전체)
	Done  bool       `json:"done"`
	OnMac bool       `json:"onMac"` // 맥에 실제로 있는지 (지우기 전에 확인)
	Dur   float64    `json:"dur,omitempty"`
	T     string     `json:"t"` // 썸네일에 쓸 파일 id
	Files []*FileRef `json:"-"`
}

var imageExt = map[string]bool{".heic": true, ".heif": true, ".jpg": true, ".jpeg": true, ".png": true, ".dng": true, ".gif": true,
	".tif": true, ".tiff": true, ".webp": true, ".raw": true, ".avif": true}
var videoExt = map[string]bool{".mov": true, ".mp4": true, ".m4v": true, ".hevc": true, ".3gp": true, ".avi": true}

// 사진이 아닌 곁다리 파일 (편집 정보 등)은 목록에 넣지 않아요
var skipExt = map[string]bool{".aae": true, ".xmp": true, ".thm": true, ".plist": true}

func fileID(it RawItem) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d|%d", it.Folder, it.Name, it.Size, int64(it.Created))))
	return hex.EncodeToString(h[:8])
}

type Library struct {
	dev     Device
	journal *Journal

	rev   atomic.Int64
	mu    sync.Mutex
	built int64
	keep  bool

	assets  []*Asset
	byID    map[string]*Asset
	files   map[string]*FileRef
	fileAst map[string]*Asset
}

func NewLibrary(dev Device, j *Journal) *Library {
	l := &Library{dev: dev, journal: j, built: -1}
	l.rev.Store(1)
	return l
}

func (l *Library) Changed() { l.rev.Add(1) }
func (l *Library) Rev() int64 { return l.rev.Load() }

// ensure rebuilds the asset list when something changed (device, journal, settings).
func (l *Library) ensure() {
	rev := l.rev.Load()
	s := app.Settings()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.built == rev && l.keep == s.KeepLive {
		return
	}
	l.build(s)
	l.built, l.keep = rev, s.KeepLive
}

func (l *Library) build(s Settings) {
	items := l.dev.Items()
	groups := map[string][]*FileRef{}
	files := map[string]*FileRef{}
	for _, it := range items {
		ext := strings.ToLower(filepath.Ext(it.Name))
		if skipExt[ext] || it.Name == "" {
			continue
		}
		f := &FileRef{ID: fileID(it), H: it.H, Name: it.Name, Size: it.Size, Created: int64(it.Created), Folder: it.Folder,
			Video: videoExt[ext], Dur: it.Dur}
		files[f.ID] = f
		stem := strings.ToUpper(strings.TrimSuffix(it.Name, filepath.Ext(it.Name)))
		key := it.Folder + "/" + stem
		groups[key] = append(groups[key], f)
	}
	dest := app.Dest()
	assets := make([]*Asset, 0, len(groups))
	byID := map[string]*Asset{}
	fileAst := map[string]*Asset{}
	add := func(fs []*FileRef, kind string) {
		sort.SliceStable(fs, func(i, j int) bool {
			if fs[i].Video != fs[j].Video {
				return !fs[i].Video
			}
			return fs[i].Name < fs[j].Name
		})
		p := fs[0]
		a := &Asset{ID: p.ID, K: kind, N: p.Name, D: p.Created, T: p.ID, Files: fs}
		for _, f := range fs {
			a.S += f.Size
			if f.Video && a.Dur == 0 {
				a.Dur = f.Dur
			}
			fileAst[f.ID] = a
		}
		if kind == "l" {
			a.Dur = 0
		}
		a.Done, a.OnMac = l.doneState(a, dest, s.KeepLive)
		assets = append(assets, a)
		byID[a.ID] = a
	}
	for _, fs := range groups {
		var imgs, vids []*FileRef
		for _, f := range fs {
			if f.Video {
				vids = append(vids, f)
			} else {
				imgs = append(imgs, f)
			}
		}
		switch {
		case len(imgs) > 0 && len(vids) == 1:
			add(append(imgs, vids...), "l")
		case len(imgs) > 0 && len(vids) == 0:
			add(imgs, "p")
		case len(imgs) == 0:
			for _, v := range vids {
				add([]*FileRef{v}, "v")
			}
		default: // 사진 여러 개 + 영상 여러 개: 따로따로
			add(imgs, "p")
			for _, v := range vids {
				add([]*FileRef{v}, "v")
			}
		}
	}
	sort.Slice(assets, func(i, j int) bool {
		if assets[i].D != assets[j].D {
			return assets[i].D > assets[j].D
		}
		return assets[i].N > assets[j].N
	})
	l.assets, l.byID, l.files, l.fileAst = assets, byID, files, fileAst
}

// required: 이 묶음에서 옮겨야 하는 파일 (Live Photo 영상은 설정에 따라)
func required(a *Asset, keepLive bool) []*FileRef {
	if a.K == "l" && !keepLive {
		var out []*FileRef
		for _, f := range a.Files {
			if !f.Video {
				out = append(out, f)
			}
		}
		return out
	}
	return a.Files
}

// doneState: 옮긴 기록이 있으면 '이미 옮김'. 기록이 없어도 정리될 자리에 같은 크기 파일이 있으면 옮긴 것으로 봐요.
// onMac: 기록된 자리(또는 정리될 자리)에 파일이 실제로 있는지
func (l *Library) doneState(a *Asset, dest string, keepLive bool) (done, onMac bool) {
	done, onMac = true, true
	for _, f := range required(a, keepLive) {
		if r, ok := l.journal.Get(f.ID); ok {
			if !fileExists(r.Dest) {
				onMac = false
			}
			continue
		}
		p := filepath.Join(dest, dateDir(a.D), f.Name)
		if st, err := os.Stat(p); err == nil && st.Size() == f.Size {
			continue
		}
		done, onMac = false, false
	}
	return
}

func dateDir(unix int64) string {
	t := time.Unix(unix, 0)
	return filepath.Join(fmt.Sprintf("%04d", t.Year()), fmt.Sprintf("%02d", int(t.Month())), fmt.Sprintf("%02d", t.Day()))
}

func (l *Library) Snapshot(s Settings) map[string]any {
	l.ensure()
	l.mu.Lock()
	defer l.mu.Unlock()
	counts := map[string]int{"all": len(l.assets)}
	var newBytes int64
	for _, a := range l.assets {
		if a.Done {
			counts["done"]++
			if !a.OnMac {
				counts["gone"]++ // 옮겼지만 맥에서 없어짐 (지웠거나 다른 곳으로 옮김)
			}
		} else {
			counts["new"]++
			newBytes += a.S
		}
		counts[a.K]++
	}
	return map[string]any{"rev": l.built, "assets": l.assets, "counts": counts, "newBytes": newBytes, "device": l.dev.Status()}
}

func (l *Library) Asset(id string) (*Asset, bool) {
	l.ensure()
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.byID[id]
	return a, ok
}

// File: 지금 연결된 iPhone 에서 이 id 의 파일 (손잡이는 연결이 바뀌면 달라져요)
func (l *Library) File(id string) (*FileRef, bool) {
	l.ensure()
	l.mu.Lock()
	defer l.mu.Unlock()
	f, ok := l.files[id]
	return f, ok
}

// FileLoose: id 로 못 찾을 때 이름 · 크기가 같은 파일이 하나뿐이면 그것으로 봐요
// (다시 연결했을 때 iPhone 이 찍은 날짜를 다르게 알려 줘도 이어서 할 수 있게)
func (l *Library) FileLoose(name string, size int64) (*FileRef, bool) {
	l.ensure()
	l.mu.Lock()
	defer l.mu.Unlock()
	var found *FileRef
	for _, f := range l.files {
		if f.Name == name && f.Size == size {
			if found != nil {
				return nil, false
			}
			found = f
		}
	}
	return found, found != nil
}

func (l *Library) Assets() []*Asset {
	l.ensure()
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*Asset(nil), l.assets...)
}
