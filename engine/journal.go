package main

// 가져온 기록 (Support/imported.jsonl) — 한 줄에 파일 하나.
// '이미 옮긴 항목 건너뛰기'와 '맥에 있는 것만 지우기'가 이 기록을 봐요.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Record struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Created int64  `json:"created"`
	Dest    string `json:"dest"`
	Sha     string `json:"sha,omitempty"`
	At      int64  `json:"at"`
	Deleted bool   `json:"del,omitempty"` // iPhone 에서 지움
}

type Journal struct {
	path string
	mu   sync.Mutex
	recs map[string]Record
	f    *os.File
	sync time.Time
}

func OpenJournal(support string) *Journal {
	j := &Journal{path: filepath.Join(support, "imported.jsonl"), recs: map[string]Record{}}
	if f, err := os.Open(j.path); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var r Record
			if json.Unmarshal(sc.Bytes(), &r) == nil && r.ID != "" {
				j.recs[r.ID] = r
			}
		}
		f.Close()
	}
	return j
}

func (j *Journal) Get(id string) (Record, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.recs[id]
	return r, ok
}

func (j *Journal) Count() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.recs)
}

// append: 바로 써요(앱이 갑자기 꺼져도 남아요). 디스크 동기화는 2초에 한 번 · 지우기 전 · 작업이 끝날 때만 (Flush).
func (j *Journal) append(rs ...Record) error {
	if j.f == nil {
		f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		j.f = f
	}
	var buf []byte
	for _, r := range rs {
		b, _ := json.Marshal(r)
		buf = append(append(buf, b...), '\n')
	}
	if _, err := j.f.Write(buf); err != nil {
		j.f.Close()
		j.f = nil
		return err
	}
	if time.Since(j.sync) > 2*time.Second {
		j.f.Sync()
		j.sync = time.Now()
	}
	return nil
}

// Flush: 기록을 디스크까지 확실히 내려보내요
func (j *Journal) Flush() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f != nil {
		j.f.Sync()
		j.sync = time.Now()
	}
}

func (j *Journal) Add(r Record) error {
	if r.At == 0 {
		r.At = time.Now().Unix()
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.append(r); err != nil {
		return err
	}
	j.recs[r.ID] = r
	return nil
}

func (j *Journal) MarkDeleted(ids []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var rs []Record
	for _, id := range ids {
		if r, ok := j.recs[id]; ok && !r.Deleted {
			r.Deleted = true
			j.recs[id] = r
			rs = append(rs, r)
		}
	}
	if len(rs) > 0 {
		j.append(rs...)
	}
}
