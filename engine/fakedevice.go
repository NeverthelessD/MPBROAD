package main

// 시험용 가짜 iPhone: 폴더 하나를 iPhone 처럼 다뤄요 (리눅스 시험 · 맥 빌드 서버 시험)
//   폴더가 없으면      → iPhone 없음 (케이블 빠짐)
//   폴더/.locked 가 있으면 → 잠김
//   폴더/.name         → iPhone 이름
//   폴더/.rate         → 초당 바이트 (천천히 보내서 잠김 · 이어 받기 시험)
//   찍은 날짜          → 파일 수정 시각

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fakeDev struct {
	root    string
	mu      sync.Mutex
	state   string
	name    string
	items   map[string]RawItem
	sig     string
	changed func()
	stop    chan struct{}
}

func newFakeDevice(root string) *fakeDev {
	return &fakeDev{root: root, state: "connecting", items: map[string]RawItem{}, stop: make(chan struct{})}
}

func (d *fakeDev) Kind() string { return "fake" }

func (d *fakeDev) Start(changed func()) {
	d.changed = changed
	d.scan()
	go func() {
		t := time.NewTicker(700 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-d.stop:
				return
			case <-t.C:
				d.scan()
			}
		}
	}()
}

func (d *fakeDev) Stop() {
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
}

func (d *fakeDev) currentState() string {
	if !dirExists(d.root) {
		return "none"
	}
	if fileExists(filepath.Join(d.root, ".locked")) {
		return "locked"
	}
	return "ready"
}

func (d *fakeDev) scan() {
	st := d.currentState()
	name := "iPhone"
	if b, err := os.ReadFile(filepath.Join(d.root, ".name")); err == nil && strings.TrimSpace(string(b)) != "" {
		name = strings.TrimSpace(string(b))
	}
	items := map[string]RawItem{}
	var sig strings.Builder
	if st == "ready" {
		filepath.WalkDir(d.root, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasPrefix(e.Name(), ".") && p != d.root {
				if e.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if e.IsDir() {
				return nil
			}
			info, err := e.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(d.root, p)
			folder := filepath.ToSlash(filepath.Dir(rel))
			if folder == "." {
				folder = "DCIM/100APPLE"
			}
			items[rel] = RawItem{H: rel, Name: e.Name(), Size: info.Size(), Created: float64(info.ModTime().Unix()), Folder: folder}
			return nil
		})
		keys := make([]string, 0, len(items))
		for k := range items {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sig.WriteString(k + ":" + strconv.FormatInt(items[k].Size, 10) + ";")
		}
	}
	d.mu.Lock()
	changed := st != d.state || name != d.name || sig.String() != d.sig
	d.state, d.name, d.items, d.sig = st, name, items, sig.String()
	d.mu.Unlock()
	if changed && d.changed != nil {
		d.changed()
	}
}

func (d *fakeDev) Status() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	pct := 0
	if d.state == "ready" {
		pct = 100
	}
	return map[string]any{"state": d.state, "name": d.name, "count": len(d.items), "catalogPct": pct, "helper": "fake", "canDelete": true, "kind": "fake"}
}

func (d *fakeDev) State() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

func (d *fakeDev) Items() []RawItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]RawItem, 0, len(d.items))
	for _, it := range d.items {
		out = append(out, it)
	}
	return out
}

func (d *fakeDev) src(h string) (string, error) {
	switch d.currentState() {
	case "none":
		return "", ErrGone
	case "locked":
		return "", ErrLocked
	}
	p := filepath.Join(d.root, filepath.FromSlash(h))
	if !strings.HasPrefix(p, filepath.Clean(d.root)+string(filepath.Separator)) || !fileExists(p) {
		return "", ErrMissing
	}
	return p, nil
}

func (d *fakeDev) Thumb(h, dst string, max int) error {
	p, err := d.src(h)
	if err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	out := scaleDown(img, max)
	w, err := os.Create(dst + ".tmp")
	if err != nil {
		return err
	}
	if err := jpeg.Encode(w, out, &jpeg.Options{Quality: 82}); err != nil {
		w.Close()
		return err
	}
	w.Close()
	return os.Rename(dst+".tmp", dst)
}

// scaleDown: 긴 변이 max 가 되게 줄여요 (상자 평균)
func scaleDown(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	var nw, nh int
	if w >= h {
		nw, nh = max, h*max/w
	} else {
		nw, nh = w*max/h, max
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint64
			for yy := y0; yy < y1; yy += 1 + (y1-y0)/4 {
				for xx := x0; xx < x1; xx += 1 + (x1-x0)/4 {
					cr, cg, cb, _ := src.At(xx, yy).RGBA()
					r, g, bl, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), 255
		}
	}
	return dst
}

func (d *fakeDev) rate() int64 {
	if v := env("FAKE_RATE"); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	if b, err := os.ReadFile(filepath.Join(d.root, ".rate")); err == nil {
		n, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		return n
	}
	return 0
}

func (d *fakeDev) Fetch(ctx context.Context, h, path string, off, size int64, progress func(int64)) (string, error) {
	p, err := d.src(h)
	if err != nil {
		return "", err
	}
	in, err := os.Open(p)
	if err != nil {
		return "", ErrMissing
	}
	defer in.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if err := out.Truncate(off); err != nil {
		out.Close()
		return "", err
	}
	if _, err := out.Seek(off, io.SeekStart); err != nil {
		out.Close()
		return "", err
	}
	if _, err := in.Seek(off, io.SeekStart); err != nil {
		out.Close()
		return "", err
	}
	buf := make([]byte, 256<<10)
	done := off
	rate := d.rate()
	t0 := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return "", err
		}
		switch d.currentState() {
		case "none":
			out.Close()
			return "", ErrGone
		case "locked":
			out.Close()
			return "", ErrLocked
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return "", werr
			}
			done += int64(n)
			if progress != nil {
				progress(done)
			}
			if rate > 0 {
				want := time.Duration(float64(done-off) / float64(rate) * float64(time.Second))
				if el := time.Since(t0); el < want {
					time.Sleep(want - el)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return "", rerr
		}
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return sha256Of(path)
}

func (d *fakeDev) Delete(hs []string) ([]string, error) {
	var out []string
	for _, h := range hs {
		p, err := d.src(h)
		if err != nil {
			if err == ErrMissing {
				continue
			}
			return out, err
		}
		if os.Remove(p) == nil {
			out = append(out, h)
		}
	}
	d.scan()
	return out, nil
}

func sha256Of(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
