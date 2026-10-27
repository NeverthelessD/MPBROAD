package main

// 가져오기 작업
//
//   1) 복사: iPhone → 저장 폴더/.mpbroad-partial/<id>.part (받은 만큼 남아 있어서, 멈춰도 그 자리부터 이어 받아요)
//   2) 확인: 크기가 같고, 다 받은 파일을 다시 읽은 sha256 이 iPhone 에서 받으면서 계산한 값과 같은지
//   3) 정리: 저장 폴더/연/월/일/원래이름 으로 옮기고(같은 이름이 있으면 _1, Live Photo 는 사진과 영상이 같은 이름), 찍은 날짜를 파일 날짜로
//   4) 기록: Support/imported.jsonl (이미 옮긴 항목 건너뛰기)
//
// 작업 계획은 Support/job.json, 진행은 Support/job.log 에 한 줄씩 적어요. 앱이 꺼졌다 켜져도 그 기록으로 이어서 해요.
// iPhone 이 잠기거나 케이블이 빠지면 '잠시 멈춤' 상태로 기다리다가, 잠금이 풀리거나 다시 연결되면 저절로 이어서 해요.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type JobFile struct {
	ID      string `json:"id"`
	Asset   string `json:"a"`
	Name    string `json:"n"`
	Size    int64  `json:"s"`
	Created int64  `json:"c"` // 이 파일이 들어갈 날짜 (묶음의 대표 날짜)
	Video   bool   `json:"v"`
	Kind    string `json:"k"` // 묶음 종류 p v l
}

type JobSpec struct {
	ID         string    `json:"id"`
	Created    int64     `json:"created"`
	Dest       string    `json:"dest"`
	Files      []JobFile `json:"files"`
	AskDelete  bool      `json:"askDelete"`
	AutoDelete []string  `json:"autoDelete,omitempty"` // 먼저 옮긴 뒤 지울 묶음 id (고른 사진 지우기)
}

type fileDone struct {
	St   string `json:"st"` // ok | skip | fail
	Dest string `json:"dest,omitempty"`
	Sha  string `json:"sha,omitempty"`
	Err  string `json:"err,omitempty"`
}

type logLine struct {
	F    string `json:"f,omitempty"`
	fileDone
	Ev   string   `json:"ev,omitempty"`
	Why  string   `json:"why,omitempty"`
	At   int64    `json:"at,omitempty"`
	Hs   []string `json:"ids,omitempty"`
}

type Jobs struct {
	support string
	dev     Device
	lib     *Library
	journal *Journal

	mu       sync.Mutex
	spec     *JobSpec
	done     map[string]fileDone
	stage    string // "" running paused locked waiting problem interrupted deleting done
	reason   string
	stopAt   int64
	cur      *JobFile
	curDone  int64
	running  bool
	cancel   context.CancelFunc
	finished chan struct{}
	wantGo   bool // 사용자가 '이어서 하기'를 누름
	copied   int
	verified int
	runStart time.Time
	runBytes int64 // 이번 실행에서 받은 바이트 (남은 시간 계산)
	result   map[string]any
	deleting bool
}

func NewJobs(support string, dev Device, lib *Library, j *Journal) *Jobs {
	js := &Jobs{support: support, dev: dev, lib: lib, journal: j, done: map[string]fileDone{}}
	js.loadInterrupted()
	return js
}

func (j *Jobs) specPath() string { return filepath.Join(j.support, "job.json") }
func (j *Jobs) logPath() string  { return filepath.Join(j.support, "job.log") }

func partialDir(dest string) string { return filepath.Join(dest, ".mpbroad-partial") }
func (j *Jobs) partPath(id string) string {
	return filepath.Join(partialDir(j.spec.Dest), id+".part")
}

// ---------------------------------------------------------------- 지난번 작업

func (j *Jobs) loadInterrupted() {
	b, err := os.ReadFile(j.specPath())
	if err != nil {
		return
	}
	var spec JobSpec
	if json.Unmarshal(b, &spec) != nil || len(spec.Files) == 0 {
		j.removeFiles(nil)
		return
	}
	j.spec = &spec
	why := ""
	var at int64
	if f, err := os.Open(j.logPath()); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var l logLine
			if json.Unmarshal(sc.Bytes(), &l) != nil {
				continue
			}
			switch {
			case l.F != "":
				j.done[l.F] = l.fileDone
			case l.Ev == "stop":
				why, at = l.Why, l.At
			case l.Ev == "start":
				why, at = "", 0
			}
		}
		f.Close()
	}
	if j.pendingCount() == 0 {
		log.Printf("지난번 작업은 이미 끝나 있었어요 → 정리")
		j.removeFiles(&spec)
		j.spec, j.done = nil, map[string]fileDone{}
		return
	}
	if !app.Settings().ResumeJobs {
		log.Printf("'멈추면 이어서 하기'가 꺼져 있어서 지난번 작업을 버려요")
		j.removeFiles(&spec)
		j.spec, j.done = nil, map[string]fileDone{}
		return
	}
	if why == "" {
		why = "앱이 갑자기 꺼졌어요"
		if st, err := os.Stat(j.logPath()); err == nil {
			at = st.ModTime().Unix()
		}
	}
	j.stage, j.reason, j.stopAt = "interrupted", why, at
	log.Printf("지난번 가져오기가 끝나지 않았어요: 남은 파일 %d개 (%s)", j.pendingCount(), why)
}

func (j *Jobs) removeFiles(spec *JobSpec) {
	os.Remove(j.specPath())
	os.Remove(j.logPath())
	if spec != nil && spec.Dest != "" {
		os.RemoveAll(partialDir(spec.Dest))
	}
}

func (j *Jobs) pendingCount() int {
	n := 0
	if j.spec == nil {
		return 0
	}
	for _, f := range j.spec.Files {
		if _, ok := j.done[f.ID]; !ok {
			n++
		}
	}
	return n
}

func (j *Jobs) appendLog(l logLine) {
	f, err := os.OpenFile(j.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	b, _ := json.Marshal(l)
	f.Write(append(b, '\n'))
	f.Sync()
	f.Close()
}

// ---------------------------------------------------------------- 상태

func (j *Jobs) Busy() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.spec != nil && j.stage != "done" && j.stage != ""
}

func (j *Jobs) Running() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running
}

func (j *Jobs) State() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := map[string]any{"stage": j.stage, "reason": j.reason}
	if j.stage == "" {
		return out
	}
	if j.result != nil && (j.stage == "done" || j.spec == nil) {
		for k, v := range j.result {
			out[k] = v
		}
		out["deleting"] = j.deleting
		return out
	}
	if j.spec == nil {
		return out
	}
	var total, doneN, ok, skip, fail int
	var bTotal, bDone int64
	assets := map[string]bool{}
	live := map[string]bool{}
	for _, f := range j.spec.Files {
		total++
		bTotal += f.Size
		assets[f.Asset] = true
		if d, has := j.done[f.ID]; has {
			doneN++
			bDone += f.Size
			switch d.St {
			case "ok":
				ok++
			case "skip":
				skip++
			default:
				fail++
			}
			if f.Kind == "l" && d.St != "fail" {
				live[f.Asset] = true
			}
		}
	}
	if j.cur != nil {
		bDone += j.curDone
	}
	out["total"], out["doneFiles"], out["ok"], out["skipped"], out["failed"] = total, doneN, ok, skip, fail
	out["assets"] = len(assets)
	out["bytesTotal"], out["bytesDone"] = bTotal, bDone
	out["copied"], out["verified"] = ok+skip+j.copied-j.verified, ok+skip
	out["dest"] = destLabel(j.spec.Dest)
	out["destPath"] = j.spec.Dest
	out["stopAt"] = j.stopAt
	out["autoDelete"] = len(j.spec.AutoDelete)
	out["askDelete"] = j.spec.AskDelete
	if j.cur != nil {
		out["cur"] = map[string]any{"name": j.cur.Name, "kind": j.cur.Kind, "dir": filepath.ToSlash(dateDir(j.cur.Created)), "id": j.cur.ID,
			"done": j.curDone, "size": j.cur.Size}
	}
	if j.running && !j.runStart.IsZero() {
		el := time.Since(j.runStart).Seconds()
		if el > 3 && j.runBytes > 0 {
			rate := float64(j.runBytes) / el
			out["eta"] = int(float64(bTotal-bDone) / rate)
			out["rate"] = int64(rate)
		}
	}
	// 받다 만 파일
	for _, f := range j.spec.Files {
		if _, has := j.done[f.ID]; has {
			continue
		}
		if st, err := os.Stat(j.partPath(f.ID)); err == nil && st.Size() > 0 {
			out["partial"] = map[string]any{"name": f.Name, "got": st.Size(), "size": f.Size, "count": 1}
		}
		break
	}
	return out
}

func (j *Jobs) setStage(stage, reason string) {
	j.mu.Lock()
	j.stage, j.reason = stage, reason
	j.mu.Unlock()
}

// ---------------------------------------------------------------- 시작 · 멈춤

func (j *Jobs) Create(assets []*Asset, s Settings, askDelete bool, autoDelete []string) (int, error) {
	j.mu.Lock()
	if j.spec != nil && j.stage != "done" {
		j.mu.Unlock()
		return 0, errors.New("이미 가져오는 중이에요. 지금 작업이 끝난 뒤에 해 주세요")
	}
	j.mu.Unlock()
	dest := app.Dest()
	if !volumeOK(dest) {
		return 0, errors.New("저장할 디스크가 연결되어 있지 않아요. 디스크를 연결하거나 설정에서 저장 위치를 바꿔 주세요")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, fmt.Errorf("저장 폴더를 만들지 못했어요: %v", err)
	}
	sort.SliceStable(assets, func(a, b int) bool { return assets[a].D < assets[b].D })
	spec := &JobSpec{ID: randID(6), Created: time.Now().Unix(), Dest: dest, AskDelete: askDelete, AutoDelete: autoDelete}
	for _, a := range assets {
		if a.Done && s.SkipDone {
			continue
		}
		for _, f := range required(a, s.KeepLive) {
			if _, ok := j.journal.Get(f.ID); ok && s.SkipDone {
				continue
			}
			spec.Files = append(spec.Files, JobFile{ID: f.ID, Asset: a.ID, Name: f.Name, Size: f.Size, Created: a.D, Video: f.Video, Kind: a.K})
		}
	}
	if len(spec.Files) == 0 && len(autoDelete) == 0 {
		return 0, errors.New("새로 옮길 항목이 없어요. 모두 이미 맥에 있어요")
	}
	b, _ := json.MarshalIndent(spec, "", " ")
	if err := writeFileAtomic(j.specPath(), b, 0o644); err != nil {
		return 0, fmt.Errorf("작업 기록을 저장하지 못했어요: %v", err)
	}
	os.Remove(j.logPath())
	j.mu.Lock()
	j.spec, j.done, j.result = spec, map[string]fileDone{}, nil
	j.copied, j.verified = 0, 0
	j.stage, j.reason, j.wantGo = "running", "", true
	j.mu.Unlock()
	log.Printf("가져오기 시작: 파일 %d개 → %s", len(spec.Files), dest)
	j.start()
	return len(spec.Files), nil
}

func (j *Jobs) start() {
	j.mu.Lock()
	if j.running || j.spec == nil {
		j.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.running, j.cancel, j.finished = true, cancel, make(chan struct{})
	j.runStart, j.runBytes = time.Now(), 0
	fin := j.finished
	j.mu.Unlock()
	j.appendLog(logLine{Ev: "start", At: time.Now().Unix()})
	go func() {
		defer close(fin)
		defer recoverLog("job run")
		j.run(ctx)
		app.lib.Changed()
		j.mu.Lock()
		j.running, j.cancel, j.cur, j.curDone = false, nil, nil, 0
		j.mu.Unlock()
	}()
}

// stopRunner: 돌고 있으면 멈추고 끝날 때까지 기다려요
func (j *Jobs) stopRunner() {
	j.mu.Lock()
	c, fin := j.cancel, j.finished
	j.mu.Unlock()
	if c != nil {
		c()
		select {
		case <-fin:
		case <-time.After(5 * time.Second):
		}
	}
}

func (j *Jobs) Pause() {
	j.stopRunner()
	j.mu.Lock()
	if j.spec != nil && j.stage != "done" && j.stage != "interrupted" {
		j.stage, j.reason, j.wantGo = "paused", "잠시 멈췄어요", false
	}
	j.mu.Unlock()
	j.appendLog(logLine{Ev: "stop", Why: "잠시 멈춤을 눌렀어요", At: time.Now().Unix()})
}

func (j *Jobs) Resume() error {
	j.mu.Lock()
	if j.spec == nil {
		j.mu.Unlock()
		return errors.New("이어서 할 작업이 없어요")
	}
	if !volumeOK(j.spec.Dest) {
		j.mu.Unlock()
		return errors.New("저장할 디스크가 연결되어 있지 않아요")
	}
	j.wantGo = true
	if !j.running {
		j.stage, j.reason = "running", ""
	}
	j.mu.Unlock()
	j.start()
	return nil
}

func (j *Jobs) Cancel() {
	j.stopRunner()
	j.mu.Lock()
	spec := j.spec
	finishedSome := len(j.done) > 0
	j.spec, j.done, j.stage, j.reason, j.result = nil, map[string]fileDone{}, "", "", nil
	j.mu.Unlock()
	if spec != nil {
		j.removeFiles(spec)
		log.Printf("가져오기 취소 (옮긴 것은 그대로 둠: %v)", finishedSome)
	}
	app.lib.Changed()
}

func (j *Jobs) Discard() { j.Cancel() }

func (j *Jobs) Dismiss() {
	j.mu.Lock()
	if j.stage == "done" && !j.deleting {
		j.stage, j.reason, j.result = "", "", nil
	}
	j.mu.Unlock()
}

func (j *Jobs) Shutdown(why string) {
	j.mu.Lock()
	running := j.running
	j.mu.Unlock()
	if !running {
		return
	}
	j.stopRunner()
	j.appendLog(logLine{Ev: "stop", Why: why, At: time.Now().Unix()})
}

// ---------------------------------------------------------------- 실행

func (j *Jobs) run(ctx context.Context) {
	suffix := map[string]string{} // 묶음 id → 이름 뒤에 붙일 _n
	for {
		if ctx.Err() != nil {
			return
		}
		j.mu.Lock()
		var next *JobFile
		var siblings []JobFile
		for i := range j.spec.Files {
			f := &j.spec.Files[i]
			if _, ok := j.done[f.ID]; !ok {
				next = f
				break
			}
		}
		if next != nil {
			for _, f := range j.spec.Files {
				if f.Asset == next.Asset {
					siblings = append(siblings, f)
				}
			}
		}
		spec := j.spec
		j.mu.Unlock()
		if next == nil {
			j.finish(ctx)
			return
		}
		res, err := j.processFile(ctx, spec, next, siblings, suffix)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("작업이 멈췄어요: %v", err)
			return
		}
		j.mu.Lock()
		j.done[next.ID] = res
		j.cur, j.curDone = nil, 0
		j.mu.Unlock()
		j.appendLog(logLine{F: next.ID, fileDone: res})
		app.lib.Changed()
	}
}

// waitReady: iPhone 이 연결되고 잠금이 풀릴 때까지 기다려요
func (j *Jobs) waitReady(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		st := j.dev.State()
		s := app.Settings()
		j.mu.Lock()
		wantGo := j.wantGo
		j.mu.Unlock()
		switch st {
		case "ready":
			if !wantGo && !s.AutoResume {
				j.setStage("locked", "iPhone 잠금이 풀렸어요. '이어서 하기'를 눌러 주세요")
				break
			}
			j.mu.Lock()
			if j.stage != "running" {
				log.Printf("iPhone 준비됨 → 이어서 해요")
			}
			j.stage, j.reason = "running", ""
			j.mu.Unlock()
			return nil
		case "locked":
			j.mu.Lock()
			if j.stage != "locked" {
				log.Printf("iPhone 잠김 → 잠시 멈춤")
				j.wantGo = false
			}
			j.stage, j.reason = "locked", "iPhone이 잠겨서 잠시 멈췄어요"
			j.mu.Unlock()
		case "loading":
			j.setStage("waiting", "iPhone 사진 목록을 불러오는 중이에요")
		default:
			j.mu.Lock()
			if j.stage != "waiting" {
				log.Printf("iPhone 연결 끊김 → 기다림")
			}
			j.stage, j.reason = "waiting", "iPhone 연결이 끊겼어요. 케이블을 다시 연결해 주세요"
			j.mu.Unlock()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func isNoSpace(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.ENOSPC
}

func (j *Jobs) problem(ctx context.Context, why string) error {
	j.setStage("problem", why)
	log.Printf("문제: %s", why)
	j.appendLog(logLine{Ev: "stop", Why: why, At: time.Now().Unix()})
	// 저장 디스크가 돌아오면 저절로 이어서 해요
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		j.mu.Lock()
		dest := j.spec.Dest
		j.mu.Unlock()
		if volumeOK(dest) && strings.Contains(why, "디스크가 연결") {
			j.setStage("running", "")
			return nil
		}
	}
}

func (j *Jobs) processFile(ctx context.Context, spec *JobSpec, f *JobFile, siblings []JobFile, suffix map[string]string) (fileDone, error) {
	tries := 0
	for {
		if err := j.waitReady(ctx); err != nil {
			return fileDone{}, err
		}
		if !volumeOK(spec.Dest) {
			if err := j.problem(ctx, "저장할 디스크가 연결되어 있지 않아요. 디스크를 다시 연결하면 이어서 해요"); err != nil {
				return fileDone{}, err
			}
			continue
		}
		ref, ok := j.lib.File(f.ID)
		if !ok {
			if j.dev.State() == "ready" {
				return fileDone{St: "fail", Err: "iPhone에서 찾을 수 없어요 (지워졌을 수 있어요)"}, nil
			}
			continue
		}
		dir := filepath.Join(spec.Dest, dateDir(f.Created))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			if isNoSpace(err) {
				return fileDone{}, j.problem(ctx, "맥 저장 공간이 부족해요. 공간을 비운 뒤 '이어서 하기'를 눌러 주세요")
			}
			return fileDone{St: "fail", Err: "폴더를 만들지 못했어요"}, nil
		}
		// 같은 묶음(Live Photo)은 같은 이름 꼬리(_1 …)를 써요
		sfx, has := suffix[f.Asset]
		if !has {
			sfx = j.chooseSuffix(dir, f, siblings)
			suffix[f.Asset] = sfx
		}
		target := filepath.Join(dir, addSuffix(f.Name, sfx))
		if st, err := os.Stat(target); err == nil {
			if st.Size() == f.Size {
				// 이미 같은 파일이 있어요 → 다시 옮기지 않아요
				j.journal.Add(Record{ID: f.ID, Name: f.Name, Size: f.Size, Created: f.Created, Dest: target})
				return fileDone{St: "skip", Dest: target}, nil
			}
			sfx = j.chooseSuffix(dir, f, siblings)
			suffix[f.Asset] = sfx
			target = filepath.Join(dir, addSuffix(f.Name, sfx))
		}

		os.MkdirAll(partialDir(spec.Dest), 0o755)
		part := filepath.Join(partialDir(spec.Dest), f.ID+".part")
		var off int64
		if st, err := os.Stat(part); err == nil {
			off = st.Size()
			if off > f.Size {
				os.Remove(part)
				off = 0
			}
		}
		j.mu.Lock()
		j.cur, j.curDone = f, off
		j.mu.Unlock()
		if off > 0 {
			log.Printf("이어 받기: %s (%d / %d)", f.Name, off, f.Size)
		}
		var last int64 = off
		sha, err := j.dev.Fetch(ctx, ref.H, part, off, f.Size, func(n int64) {
			j.mu.Lock()
			if n > last {
				j.runBytes += n - last
				last = n
			}
			j.curDone = n
			j.mu.Unlock()
		})
		if err != nil {
			if ctx.Err() != nil {
				return fileDone{}, ctx.Err()
			}
			switch {
			case errors.Is(err, ErrLocked), errors.Is(err, ErrGone):
				log.Printf("%s 받다가 멈춤: %v", f.Name, err)
				time.Sleep(time.Second)
				continue
			case errors.Is(err, ErrMissing):
				return fileDone{St: "fail", Err: ErrMissing.Error()}, nil
			case isNoSpace(err):
				if e := j.problem(ctx, "맥 저장 공간이 부족해요. 공간을 비운 뒤 '이어서 하기'를 눌러 주세요"); e != nil {
					return fileDone{}, e
				}
				continue
			}
			tries++
			log.Printf("%s 받기 실패 (%d번째): %v", f.Name, tries, err)
			if tries >= 3 {
				return fileDone{St: "fail", Err: err.Error()}, nil
			}
			time.Sleep(time.Duration(tries) * time.Second)
			continue
		}
		j.mu.Lock()
		j.copied++
		j.mu.Unlock()
		// 확인: 크기 + 내용
		st, err := os.Stat(part)
		got, herr := sha256Of(part)
		if err != nil || st.Size() != f.Size || herr != nil || (sha != "" && !strings.EqualFold(sha, got)) {
			log.Printf("확인 실패: %s (크기 %v/%d, sha %s/%s)", f.Name, sizeOf(st), f.Size, sha, got)
			os.Remove(part)
			j.mu.Lock()
			j.copied--
			j.mu.Unlock()
			tries++
			if tries >= 3 {
				return fileDone{St: "fail", Err: "복사본이 원본과 달라요"}, nil
			}
			continue
		}
		if err := os.Rename(part, target); err != nil {
			if isNoSpace(err) {
				return fileDone{}, j.problem(ctx, "맥 저장 공간이 부족해요")
			}
			return fileDone{St: "fail", Err: "정리할 자리로 옮기지 못했어요"}, nil
		}
		t := time.Unix(f.Created, 0)
		os.Chtimes(target, t, t)
		if err := j.journal.Add(Record{ID: f.ID, Name: f.Name, Size: f.Size, Created: f.Created, Dest: target, Sha: got}); err != nil {
			log.Printf("기록 실패: %v", err)
		}
		j.mu.Lock()
		j.verified++
		j.mu.Unlock()
		return fileDone{St: "ok", Dest: target, Sha: got}, nil
	}
}

func sizeOf(st os.FileInfo) int64 {
	if st == nil {
		return -1
	}
	return st.Size()
}

func addSuffix(name, sfx string) string {
	if sfx == "" {
		return name
	}
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + sfx + ext
}

// chooseSuffix: 묶음의 모든 파일 이름이 비어 있는 자리(또는 같은 파일)인 가장 작은 꼬리를 골라요
func (j *Jobs) chooseSuffix(dir string, f *JobFile, siblings []JobFile) string {
	// 같은 묶음 중 이미 끝난 파일이 있으면 그 이름을 따라가요
	j.mu.Lock()
	for _, s := range siblings {
		if d, ok := j.done[s.ID]; ok && d.Dest != "" {
			base := filepath.Base(d.Dest)
			stem := strings.TrimSuffix(s.Name, filepath.Ext(s.Name))
			got := strings.TrimSuffix(base, filepath.Ext(base))
			if strings.HasPrefix(got, stem) {
				j.mu.Unlock()
				return strings.TrimPrefix(got, stem)
			}
		}
	}
	j.mu.Unlock()
	for n := 0; n < 10000; n++ {
		sfx := ""
		if n > 0 {
			sfx = fmt.Sprintf("_%d", n)
		}
		free := true
		for _, s := range siblings {
			if st, err := os.Stat(filepath.Join(dir, addSuffix(s.Name, sfx))); err == nil && st.Size() != s.Size {
				free = false
				break
			}
		}
		if free {
			return sfx
		}
	}
	return "_" + randID(3)
}

// ---------------------------------------------------------------- 끝 · 지우기

func (j *Jobs) finish(ctx context.Context) {
	j.mu.Lock()
	spec := j.spec
	res := j.summaryLocked()
	j.mu.Unlock()
	log.Printf("가져오기 끝: %v", res)
	auto := spec.AutoDelete
	if len(auto) > 0 {
		j.mu.Lock()
		j.stage, j.reason, j.deleting = "deleting", "", true
		j.mu.Unlock()
		del, fail, err := j.deleteAssets(ctx, auto)
		res["deleted"], res["deleteFailed"] = del, fail
		if err != nil {
			res["deleteError"] = err.Error()
		}
		res["kind"] = "importDelete"
	} else {
		res["kind"] = "import"
		res["askDelete"] = spec.AskDelete && res["deletable"].(int) > 0
	}
	j.removeFiles(spec)
	j.mu.Lock()
	j.result, j.stage, j.reason, j.deleting = res, "done", "", false
	j.spec, j.done = nil, map[string]fileDone{}
	j.mu.Unlock()
	app.lib.Changed()
}

func (j *Jobs) summaryLocked() map[string]any {
	var ok, skip, fail int
	var bytes, freeBytes int64
	assets := map[string]bool{}
	live := map[string]bool{}
	deletable := map[string]bool{}
	failedAsset := map[string]bool{}
	var failedNames []string
	for _, f := range j.spec.Files {
		d := j.done[f.ID]
		assets[f.Asset] = true
		switch d.St {
		case "ok":
			ok++
			bytes += f.Size
			freeBytes += f.Size
		case "skip":
			skip++
			freeBytes += f.Size
		default:
			fail++
			failedAsset[f.Asset] = true
			if len(failedNames) < 20 {
				failedNames = append(failedNames, f.Name+" — "+d.Err)
			}
		}
		if f.Kind == "l" && f.Video && d.St != "fail" {
			live[f.Asset] = true
		}
	}
	for a := range assets {
		if !failedAsset[a] {
			deletable[a] = true
		}
	}
	ids := make([]string, 0, len(deletable))
	for a := range deletable {
		ids = append(ids, a)
	}
	return map[string]any{"assets": len(assets), "files": len(j.spec.Files), "ok": ok, "skipped": skip, "failed": fail, "bytes": bytes, "freeBytes": freeBytes,
		"livePairs": len(live), "deletable": len(deletable), "deletableIDs": ids, "failedNames": failedNames,
		"dest": destLabel(j.spec.Dest), "destPath": j.spec.Dest}
}

// DeleteVerified: 방금 가져오기에서 확인이 끝난 묶음을 iPhone 에서 지워요 (④ 'iPhone에서 지우기')
func (j *Jobs) DeleteVerified() error {
	j.mu.Lock()
	if j.stage != "done" || j.result == nil || j.deleting {
		j.mu.Unlock()
		return errors.New("지울 항목이 없어요")
	}
	ids, _ := j.result["deletableIDs"].([]string)
	j.deleting = true
	j.mu.Unlock()
	go func() {
		defer recoverLog("delete verified")
		del, fail, err := j.deleteAssets(context.Background(), ids)
		app.lib.Changed()
		j.mu.Lock()
		j.deleting = false
		if j.result != nil {
			j.result["deleted"], j.result["deleteFailed"], j.result["askDelete"] = del, fail, false
			j.result["kind"] = "deleted"
			if err != nil {
				j.result["deleteError"] = err.Error()
			}
		}
		j.mu.Unlock()
	}()
	return nil
}

// deleteAssets: 맥에 확인된 파일만 iPhone 에서 지워요. 하나라도 맥에 없는 파일이 있는 묶음은 그 파일을 남겨요.
func (j *Jobs) deleteAssets(ctx context.Context, assetIDs []string) (int, int, error) {
	return j.deleteMode(ctx, assetIDs, true)
}

// deleteMode: onlyOnMac 이면 맥에 확인된 파일만, 아니면 묶음의 모든 파일을 지워요
func (j *Jobs) deleteMode(ctx context.Context, assetIDs []string, onlyOnMac bool) (int, int, error) {
	if st := j.dev.State(); st != "ready" {
		if st == "locked" {
			return 0, len(assetIDs), errors.New("iPhone이 잠겨 있어서 지우지 못했어요. 잠금을 풀고 다시 해 주세요")
		}
		return 0, len(assetIDs), errors.New("iPhone이 연결되어 있지 않아서 지우지 못했어요")
	}
	var hs []string
	hToID := map[string]string{}
	assetOf := map[string]string{}
	want := map[string]int{}
	for _, id := range assetIDs {
		a, ok := j.lib.Asset(id)
		if !ok {
			continue
		}
		for _, f := range a.Files {
			if onlyOnMac {
				r, ok := j.journal.Get(f.ID)
				if !ok {
					continue
				}
				if st, err := os.Stat(r.Dest); err != nil || st.Size() != f.Size {
					continue
				}
			}
			hs = append(hs, f.H)
			hToID[f.H] = f.ID
			assetOf[f.H] = a.ID
			want[a.ID]++
		}
	}
	if len(hs) == 0 {
		return 0, len(assetIDs), nil
	}
	var deletedIDs []string
	gotAsset := map[string]int{}
	var lastErr error
	for i := 0; i < len(hs); i += 200 {
		if ctx.Err() != nil {
			break
		}
		end := min(i+200, len(hs))
		got, err := j.dev.Delete(hs[i:end])
		for _, h := range got {
			deletedIDs = append(deletedIDs, hToID[h])
			gotAsset[assetOf[h]]++
		}
		if err != nil {
			lastErr = err
			log.Printf("지우기 실패: %v", err)
			break
		}
	}
	j.journal.MarkDeleted(deletedIDs)
	n := 0
	for a, c := range want {
		if gotAsset[a] == c {
			n++
		}
	}
	log.Printf("iPhone에서 지움: 묶음 %d개 (파일 %d개)", n, len(deletedIDs))
	app.lib.Changed()
	if lastErr != nil {
		return n, len(assetIDs) - n, errors.New("일부를 지우지 못했어요: " + lastErr.Error())
	}
	return n, len(assetIDs) - n, nil
}

// DeleteDirect: 옮기지 않고 바로 지우기
func (j *Jobs) DeleteDirect(assetIDs []string) error {
	j.mu.Lock()
	if j.spec != nil && j.stage != "done" {
		j.mu.Unlock()
		return errors.New("가져오는 중에는 지울 수 없어요. 끝난 뒤에 해 주세요")
	}
	if j.deleting {
		j.mu.Unlock()
		return errors.New("지우는 중이에요")
	}
	if st := j.dev.State(); st != "ready" {
		j.mu.Unlock()
		if st == "locked" {
			return errors.New("iPhone 잠금을 풀어 주세요")
		}
		return errors.New("iPhone이 연결되어 있지 않아요")
	}
	j.stage, j.reason, j.deleting, j.result = "deleting", "", true, map[string]any{"kind": "deleted", "assets": len(assetIDs)}
	j.mu.Unlock()
	go func() {
		defer recoverLog("delete direct")
		del, fail, err := j.deleteMode(context.Background(), assetIDs, false)
		app.lib.Changed()
		j.mu.Lock()
		j.deleting = false
		j.stage = "done"
		j.result["deleted"], j.result["deleteFailed"] = del, fail
		if err != nil {
			j.result["deleteError"] = err.Error()
		}
		j.mu.Unlock()
	}()
	return nil
}

// ---------------------------------------------------------------- API

func (a *App) assetsFor(ids []string, all bool) []*Asset {
	var out []*Asset
	if all {
		for _, x := range a.lib.Assets() {
			if !x.Done {
				out = append(out, x)
			}
		}
		return out
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if x, ok := a.lib.Asset(id); ok {
			out = append(out, x)
		}
	}
	return out
}

func notReadyMsg(st string) string {
	switch st {
	case "ready":
		return ""
	case "locked":
		return "iPhone 잠금을 풀어 주세요"
	case "loading":
		return "iPhone 사진 목록을 불러오는 중이에요. 잠시 뒤에 다시 해 주세요"
	}
	return "iPhone이 연결되어 있지 않아요"
}

func (a *App) handleImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs       []string `json:"ids"`
		All       bool     `json:"all"`
		AskDelete bool     `json:"askDelete"`
		Redo      bool     `json:"redo"` // 이미 옮긴 항목도 다시 옮기기 (맥에서 지웠을 때)
	}
	if err := readJSON(r, &in); err != nil {
		jsonError(w, 400, "bad request")
		return
	}
	if msg := notReadyMsg(a.dev.State()); msg != "" {
		jsonError(w, 400, msg)
		return
	}
	assets := a.assetsFor(in.IDs, in.All)
	s := a.Settings()
	if in.Redo {
		// 기록은 보지 않고 다시 옮겨요. 정리될 자리에 같은 파일이 그대로 있으면 그건 건너뛰어요(사본을 만들지 않아요).
		s.SkipDone = false
	}
	n, err := a.jobs.Create(assets, s, in.AskDelete, nil)
	if err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "files": n, "job": a.jobs.State()})
}

func (a *App) handleDeletePlan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	readJSON(r, &in)
	out := map[string]int{}
	var bytes, needBytes int64
	for _, x := range a.assetsFor(in.IDs, false) {
		out["total"]++
		out[x.K]++
		bytes += x.S
		if x.Done && x.OnMac {
			out["onMac"]++
		} else {
			out["notOnMac"]++
			needBytes += x.S
		}
	}
	writeJSON(w, map[string]any{"counts": out, "bytes": bytes, "needBytes": needBytes})
}

func (a *App) handleDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs  []string `json:"ids"`
		Mode string   `json:"mode"` // import-first | direct
	}
	if err := readJSON(r, &in); err != nil || len(in.IDs) == 0 {
		jsonError(w, 400, "지울 항목을 골라 주세요")
		return
	}
	if msg := notReadyMsg(a.dev.State()); msg != "" {
		jsonError(w, 400, msg)
		return
	}
	assets := a.assetsFor(in.IDs, false)
	var ids []string
	for _, x := range assets {
		ids = append(ids, x.ID)
	}
	if in.Mode == "direct" {
		if err := a.jobs.DeleteDirect(ids); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "job": a.jobs.State()})
		return
	}
	// 먼저 맥에 옮긴 뒤(이미 옮긴 것은 건너뛰고) 고른 것 모두 지우기
	s := a.Settings()
	s.SkipDone = true
	if _, err := a.jobs.Create(assets, s, false, ids); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "job": a.jobs.State()})
}
