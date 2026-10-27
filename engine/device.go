package main

// iPhone 연결
//
// 맥에서 iPhone 사진에 다가가려면 Apple 의 ImageCaptureCore 를 써야 하는데, Go 에서는 부를 수 없어요.
// 그래서 작은 Swift 도우미(mpbroad-device, app/device/main.swift)를 엔진의 자식으로 켜고,
// 한 줄짜리 JSON 으로 이야기해요 (엔진 → 도우미: stdin, 도우미 → 엔진: stdout, 도우미의 기록: device.log).
//
// 시험할 때는 MPB_FAKE_DEVICE=<폴더> (또는 Support/test-fake-device 파일에 폴더 경로)로 폴더를 iPhone 처럼 써요 (fakedevice.go).

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RawItem: iPhone 안의 파일 하나
type RawItem struct {
	H       string  `json:"h"`       // 도우미가 붙인 손잡이 (연결이 바뀌면 달라질 수 있음)
	Name    string  `json:"name"`    // IMG_4821.HEIC
	Size    int64   `json:"size"`    // 바이트
	Created float64 `json:"created"` // 찍은 날짜 (유닉스 초)
	UTI     string  `json:"uti"`
	Group   string  `json:"group"`  // Live Photo 등 묶음 표시 (있으면)
	Folder  string  `json:"folder"` // DCIM/100APPLE
	Dur     float64 `json:"dur"`    // 동영상 길이(초)
}

var (
	ErrLocked  = errors.New("iPhone이 잠겨 있어요")
	ErrGone    = errors.New("iPhone 연결이 끊겼어요")
	ErrMissing = errors.New("iPhone에서 이 파일을 찾을 수 없어요")
)

type Device interface {
	Kind() string
	Start(changed func())
	Stop()
	Status() map[string]any
	State() string // none | connecting | locked | loading | ready | unavailable
	Items() []RawItem
	Thumb(h, dst string, max int) error
	// Fetch 는 path 파일을 off 바이트까지 남기고 그 뒤를 이어 받아요. 다 받으면 파일 전체의 sha256 을 돌려줘요.
	Fetch(ctx context.Context, h, path string, off, size int64, progress func(done int64)) (string, error)
	Delete(hs []string) ([]string, error)
}

func newDevice(support string) Device {
	fake := env("FAKE_DEVICE")
	if fake == "" {
		if b, err := os.ReadFile(filepath.Join(support, "test-fake-device")); err == nil {
			fake = strings.TrimSpace(string(b))
		}
	}
	if fake != "" {
		log.Printf("시험용 가짜 iPhone: %s", fake)
		return newFakeDevice(fake)
	}
	helper := env("DEVICE_HELPER")
	if helper == "" {
		if exe, err := os.Executable(); err == nil {
			helper = filepath.Join(filepath.Dir(exe), "mpbroad-device")
		}
	}
	return &helperDev{support: support, path: helper, state: "connecting", helper: "starting",
		items: map[string]RawItem{}, pending: map[int64]chan map[string]any{}, progress: map[int64]func(int64){}}
}

// ---------------------------------------------------------------- Swift 도우미

type helperDev struct {
	support string
	path    string

	mu        sync.Mutex
	stdin     io.WriteCloser
	proc      *os.Process
	state     string
	name      string
	pct       int
	canDelete bool
	helper    string // starting | ok | missing | restarting
	items     map[string]RawItem
	pending   map[int64]chan map[string]any
	progress  map[int64]func(int64)
	nextID    int64
	changed   func()
	stopped   bool
	wmu       sync.Mutex
}

func (d *helperDev) Kind() string { return "iphone" }

func (d *helperDev) devlog(format string, a ...any) {
	p := filepath.Join(d.support, "device.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 512<<10 {
		os.Rename(p, filepath.Join(d.support, "device.prev.log"))
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "%s [엔진] %s\n", time.Now().Format("01-02 15:04:05"), fmt.Sprintf(format, a...))
		f.Close()
	}
}

func (d *helperDev) Start(changed func()) {
	d.changed = changed
	go d.loop()
}

func (d *helperDev) notify() {
	if d.changed != nil {
		d.changed()
	}
}

func (d *helperDev) loop() {
	defer recoverLog("device loop")
	backoff := time.Second
	for {
		d.mu.Lock()
		if d.stopped {
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
		if !fileExists(d.path) {
			d.mu.Lock()
			d.helper, d.state = "missing", "unavailable"
			d.mu.Unlock()
			d.devlog("도우미가 없어요: %s", d.path)
			d.notify()
			return
		}
		started := time.Now()
		err := d.runOnce()
		d.mu.Lock()
		stopped := d.stopped
		d.items = map[string]RawItem{}
		d.state, d.helper = "connecting", "restarting"
		for id, ch := range d.pending {
			ch <- map[string]any{"ok": false, "code": "gone", "err": "도우미가 다시 시작됐어요"}
			delete(d.pending, id)
		}
		d.mu.Unlock()
		d.notify()
		if stopped {
			return
		}
		d.devlog("도우미가 끝났어요: %v → 다시 켜요", err)
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		time.Sleep(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (d *helperDev) runOnce() error {
	cmd := exec.Command(d.path)
	cmd.Env = os.Environ()
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	lf, _ := os.OpenFile(filepath.Join(d.support, "device.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if lf != nil {
		cmd.Stderr = lf
		defer lf.Close()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	d.mu.Lock()
	d.stdin, d.proc, d.helper = in, cmd.Process, "ok"
	d.mu.Unlock()
	d.devlog("도우미 켬 (pid %d)", cmd.Process.Pid)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		d.handle(m)
	}
	in.Close()
	return cmd.Wait()
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (d *helperDev) handle(m map[string]any) {
	if re, ok := m["re"]; ok {
		id := int64(num(re))
		d.mu.Lock()
		ch := d.pending[id]
		delete(d.pending, id)
		delete(d.progress, id)
		d.mu.Unlock()
		if ch != nil {
			ch <- m
		}
		return
	}
	switch str(m["ev"]) {
	case "status":
		d.mu.Lock()
		prev := d.state
		d.state = str(m["state"])
		d.name = str(m["name"])
		d.pct = int(num(m["catalogPct"]))
		d.canDelete, _ = m["canDelete"].(bool)
		if d.state == "none" {
			d.items = map[string]RawItem{}
		}
		d.mu.Unlock()
		if prev != d.state {
			d.devlog("상태 %s → %s (%s)", prev, d.state, d.name)
		}
		d.notify()
	case "add":
		b, _ := json.Marshal(m["items"])
		var items []RawItem
		json.Unmarshal(b, &items)
		d.mu.Lock()
		for _, it := range items {
			d.items[it.H] = it
		}
		d.mu.Unlock()
		d.notify()
	case "remove":
		d.mu.Lock()
		if hs, ok := m["hs"].([]any); ok {
			for _, h := range hs {
				delete(d.items, str(h))
			}
		}
		d.mu.Unlock()
		d.notify()
	case "reset":
		d.mu.Lock()
		d.items = map[string]RawItem{}
		d.mu.Unlock()
		d.notify()
	case "progress":
		id := int64(num(m["for"]))
		d.mu.Lock()
		f := d.progress[id]
		d.mu.Unlock()
		if f != nil {
			f(int64(num(m["done"])))
		}
	case "log":
		d.devlog("도우미: %s", str(m["msg"]))
	}
}

func (d *helperDev) Stop() {
	d.mu.Lock()
	d.stopped = true
	in, p := d.stdin, d.proc
	d.mu.Unlock()
	if in != nil {
		in.Close()
	}
	if p != nil {
		done := make(chan struct{})
		go func() { p.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
			p.Kill()
		}
	}
}

func (d *helperDev) send(req map[string]any, onProgress func(int64)) (int64, chan map[string]any, error) {
	d.mu.Lock()
	if d.stdin == nil || d.helper != "ok" {
		d.mu.Unlock()
		return 0, nil, ErrGone
	}
	d.nextID++
	id := d.nextID
	ch := make(chan map[string]any, 1)
	d.pending[id] = ch
	if onProgress != nil {
		d.progress[id] = onProgress
	}
	in := d.stdin
	d.mu.Unlock()
	req["id"] = id
	b, _ := json.Marshal(req)
	d.wmu.Lock()
	_, err := in.Write(append(b, '\n'))
	d.wmu.Unlock()
	if err != nil {
		d.mu.Lock()
		delete(d.pending, id)
		delete(d.progress, id)
		d.mu.Unlock()
		return 0, nil, ErrGone
	}
	return id, ch, nil
}

func (d *helperDev) forget(id int64) {
	d.mu.Lock()
	delete(d.pending, id)
	delete(d.progress, id)
	d.mu.Unlock()
}

func respErr(m map[string]any) error {
	if ok, _ := m["ok"].(bool); ok {
		return nil
	}
	switch str(m["code"]) {
	case "locked":
		return ErrLocked
	case "gone":
		return ErrGone
	case "missing":
		return ErrMissing
	}
	if e := str(m["err"]); e != "" {
		return errors.New(e)
	}
	return errors.New("iPhone에서 읽지 못했어요")
}

func (d *helperDev) call(req map[string]any, timeout time.Duration) (map[string]any, error) {
	id, ch, err := d.send(req, nil)
	if err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		return m, respErr(m)
	case <-time.After(timeout):
		d.forget(id)
		return nil, errors.New("iPhone이 응답하지 않아요")
	}
}

func (d *helperDev) Status() map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return map[string]any{"state": d.state, "name": d.name, "count": len(d.items), "catalogPct": d.pct,
		"helper": d.helper, "canDelete": d.canDelete, "kind": "iphone"}
}

func (d *helperDev) State() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state
}

func (d *helperDev) Items() []RawItem {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]RawItem, 0, len(d.items))
	for _, it := range d.items {
		out = append(out, it)
	}
	return out
}

func (d *helperDev) Thumb(h, dst string, max int) error {
	_, err := d.call(map[string]any{"op": "thumb", "h": h, "path": dst, "max": max}, 40*time.Second)
	return err
}

func (d *helperDev) Fetch(ctx context.Context, h, path string, off, size int64, progress func(int64)) (string, error) {
	last := time.Now()
	var lmu sync.Mutex
	id, ch, err := d.send(map[string]any{"op": "fetch", "h": h, "path": path, "off": off, "size": size}, func(n int64) {
		lmu.Lock()
		last = time.Now()
		lmu.Unlock()
		if progress != nil {
			progress(n)
		}
	})
	if err != nil {
		return "", err
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case m := <-ch:
			if err := respErr(m); err != nil {
				return "", err
			}
			return str(m["sha"]), nil
		case <-ctx.Done():
			d.send(map[string]any{"op": "cancel", "target": id}, nil)
			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				d.forget(id)
			}
			return "", ctx.Err()
		case <-tick.C:
			lmu.Lock()
			idle := time.Since(last)
			lmu.Unlock()
			if idle > 120*time.Second {
				d.send(map[string]any{"op": "cancel", "target": id}, nil)
				d.forget(id)
				return "", errors.New("iPhone이 2분 동안 응답하지 않아요")
			}
			if st := d.State(); st == "none" || st == "locked" {
				// 도우미가 곧 실패를 알려 줄 거예요. 혹시 못 알려 주면 여기서 끝내요.
				if idle > 20*time.Second {
					d.send(map[string]any{"op": "cancel", "target": id}, nil)
					d.forget(id)
					if st == "locked" {
						return "", ErrLocked
					}
					return "", ErrGone
				}
			}
		}
	}
}

func (d *helperDev) Delete(hs []string) ([]string, error) {
	m, err := d.call(map[string]any{"op": "delete", "hs": hs}, 10*time.Minute)
	if m == nil {
		return nil, err
	}
	var out []string
	if arr, ok := m["deleted"].([]any); ok {
		for _, h := range arr {
			out = append(out, str(h))
		}
	}
	return out, err
}
