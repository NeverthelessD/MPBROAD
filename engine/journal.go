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

func (j *Journal) append(rs ...Record) error {
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, r := range rs {
		b, _ := json.Marshal(r)
		w.Write(append(b, '\n'))
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	f.Sync()
	return f.Close()
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
