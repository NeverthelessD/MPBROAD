package main

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testEnv struct {
	root, dest, support string
	fd                  *fakeDev
}

func setupApp(t *testing.T) *testEnv {
	t.Helper()
	base := t.TempDir()
	e := &testEnv{root: filepath.Join(base, "iphone", "DCIM", "100APPLE"), dest: filepath.Join(base, "dest"), support: filepath.Join(base, "support")}
	os.MkdirAll(e.root, 0o755)
	os.MkdirAll(e.support, 0o755)
	t.Setenv("MPB_DEFAULT_DEST", e.dest)
	app = &App{support: e.support}
	app.loadSettings()
	app.journal = OpenJournal(e.support)
	e.fd = newFakeDevice(filepath.Join(base, "iphone"))
	app.dev = e.fd
	app.lib = NewLibrary(e.fd, app.journal)
	app.jobs = NewJobs(e.support, e.fd, app.lib, app.journal)
	app.upd = NewUpdater(e.support)
	e.fd.Start(func() { app.lib.Changed() })
	t.Cleanup(func() {
		e.fd.Stop()
		app.jobs.stopRunner()
		for i := 0; i < 100; i++ {
			app.jobs.mu.Lock()
			busy := app.jobs.running || app.jobs.deleting
			app.jobs.mu.Unlock()
			if !busy {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
	})
	return e
}

func (e *testEnv) put(t *testing.T, name string, size int, when time.Time) []byte {
	t.Helper()
	b := make([]byte, size)
	rand.Read(b)
	p := filepath.Join(e.root, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, when, when)
	return b
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	dl := time.Now().Add(timeout)
	for time.Now().Before(dl) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("시간 초과: %s (작업 상태 %v)", what, app.jobs.State())
}

func stage() string { s, _ := app.jobs.State()["stage"].(string); return s }

func (e *testEnv) rescan() { e.fd.scan() }

func TestImportOrganizeLiveAndSkip(t *testing.T) {
	e := setupApp(t)
	d1 := time.Date(2026, 10, 27, 14, 0, 0, 0, time.Local)
	d2 := time.Date(2025, 3, 5, 9, 30, 0, 0, time.Local)
	photo := e.put(t, "IMG_0001.JPG", 300_000, d1)
	liveImg := e.put(t, "IMG_0002.HEIC", 200_000, d2)
	liveMov := e.put(t, "IMG_0002.MOV", 400_000, d2)
	e.put(t, "IMG_0002.AAE", 500, d2)
	e.put(t, "IMG_0003.MOV", 900_000, d1)
	e.rescan()

	snap := app.lib.Snapshot(app.Settings())
	counts := snap["counts"].(map[string]int)
	if counts["all"] != 3 || counts["l"] != 1 || counts["v"] != 1 || counts["p"] != 1 || counts["new"] != 3 {
		t.Fatalf("묶음이 이상해요: %v", counts)
	}
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), true, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "가져오기 끝", 10*time.Second, func() bool { return stage() == "done" })
	st := app.jobs.State()
	if st["ok"].(int) != 4 || st["failed"].(int) != 0 || st["livePairs"].(int) != 1 || st["askDelete"] != true {
		t.Fatalf("결과가 이상해요: %v", st)
	}
	check := func(rel string, want []byte) {
		b, err := os.ReadFile(filepath.Join(e.dest, rel))
		if err != nil || !bytes.Equal(b, want) {
			t.Fatalf("%s 가 없거나 내용이 달라요 (%v)", rel, err)
		}
	}
	check("2026/10/27/IMG_0001.JPG", photo)
	check("2025/03/05/IMG_0002.HEIC", liveImg)
	check("2025/03/05/IMG_0002.MOV", liveMov)
	if fi, _ := os.Stat(filepath.Join(e.dest, "2026/10/27/IMG_0001.JPG")); !fi.ModTime().Equal(d1) {
		t.Fatalf("파일 날짜가 찍은 날짜가 아니에요: %v", fi.ModTime())
	}
	if fileExists(filepath.Join(e.dest, "2025/03/05/IMG_0002.AAE")) {
		t.Fatal("AAE 는 옮기지 않아요")
	}
	if fileExists(partialDir(e.dest)) {
		t.Fatal("임시 폴더가 남았어요")
	}
	counts = app.lib.Snapshot(app.Settings())["counts"].(map[string]int)
	if counts["done"] != 3 || counts["new"] != 0 {
		t.Fatalf("이미 옮김 표시가 안 돼요: %v", counts)
	}
	app.jobs.Dismiss()
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), true, nil); err == nil {
		t.Fatal("다 옮겼는데 또 가져와요")
	}
	// 기록을 지워도(다른 맥 · 다시 설치) 같은 자리에 같은 크기 파일이 있으면 이미 옮김
	os.Remove(filepath.Join(e.support, "imported.jsonl"))
	app.journal = OpenJournal(e.support)
	app.lib.journal = app.journal
	app.jobs.journal = app.journal
	app.lib.Changed()
	counts = app.lib.Snapshot(app.Settings())["counts"].(map[string]int)
	if counts["done"] != 3 {
		t.Fatalf("정리된 파일을 알아보지 못해요: %v", counts)
	}

	// iPhone 에서 지우기 (확인된 것만)
	if err := app.jobs.DeleteVerified(); err == nil {
		t.Fatal("결과를 닫은 뒤에는 지울 게 없어야 해요")
	}
}

func TestCollisionKeepsLivePairNames(t *testing.T) {
	e := setupApp(t)
	d := time.Date(2026, 1, 2, 10, 0, 0, 0, time.Local)
	e.put(t, "IMG_0010.HEIC", 1000, d)
	mov := e.put(t, "IMG_0010.MOV", 2000, d)
	// 다른 날 같은 이름의 다른 사진이 이미 그 자리에 있어요 (크기가 다름)
	os.MkdirAll(filepath.Join(e.dest, "2026/01/02"), 0o755)
	os.WriteFile(filepath.Join(e.dest, "2026/01/02/IMG_0010.HEIC"), []byte("other"), 0o644)
	e.rescan()
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), false, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "끝", 10*time.Second, func() bool { return stage() == "done" })
	if !fileExists(filepath.Join(e.dest, "2026/01/02/IMG_0010_1.HEIC")) {
		t.Fatal("사진이 _1 로 들어가지 않았어요")
	}
	b, _ := os.ReadFile(filepath.Join(e.dest, "2026/01/02/IMG_0010_1.MOV"))
	if !bytes.Equal(b, mov) {
		t.Fatal("Live Photo 영상이 사진과 같은 이름(_1)이 아니에요")
	}
	if fileExists(filepath.Join(e.dest, "2026/01/02/IMG_0010.MOV")) {
		t.Fatal("영상만 다른 이름으로 들어갔어요")
	}
}

func TestLockedPausesAndAutoResumes(t *testing.T) {
	e := setupApp(t)
	d := time.Date(2026, 5, 1, 8, 0, 0, 0, time.Local)
	big := e.put(t, "IMG_0100.MOV", 3_000_000, d)
	os.WriteFile(filepath.Join(filepath.Dir(e.root), "..", ".rate"), []byte("2000000"), 0o644)
	e.rescan()
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), false, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(600 * time.Millisecond)
	lock := filepath.Join(filepath.Dir(e.root), "..", ".locked")
	os.WriteFile(lock, nil, 0o644)
	waitFor(t, "잠김 → 잠시 멈춤", 5*time.Second, func() bool { return stage() == "locked" })
	p := app.jobs.State()["partial"]
	if p == nil {
		t.Fatalf("받다 만 파일 정보가 없어요: %v", app.jobs.State())
	}
	got := p.(map[string]any)["got"].(int64)
	if got <= 0 || got >= 3_000_000 {
		t.Fatalf("받다 만 크기가 이상해요: %d", got)
	}
	os.Remove(lock)
	waitFor(t, "잠금 풀면 저절로 끝까지", 10*time.Second, func() bool { return stage() == "done" })
	b, _ := os.ReadFile(filepath.Join(e.dest, "2026/05/01/IMG_0100.MOV"))
	if !bytes.Equal(b, big) {
		t.Fatal("이어 받은 파일 내용이 원본과 달라요")
	}
}

func TestUnplugWaitsAndContinues(t *testing.T) {
	e := setupApp(t)
	d := time.Date(2026, 5, 2, 8, 0, 0, 0, time.Local)
	a := e.put(t, "IMG_0200.MOV", 2_000_000, d)
	e.put(t, "IMG_0201.JPG", 1000, d)
	os.WriteFile(filepath.Join(filepath.Dir(e.root), "..", ".rate"), []byte("2000000"), 0o644)
	e.rescan()
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), false, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	phone := filepath.Join(filepath.Dir(e.root), "..")
	os.Rename(phone, phone+".off")
	waitFor(t, "연결 끊김 → 기다림", 5*time.Second, func() bool { return stage() == "waiting" })
	os.Rename(phone+".off", phone)
	waitFor(t, "다시 연결 → 끝", 10*time.Second, func() bool { return stage() == "done" })
	b, _ := os.ReadFile(filepath.Join(e.dest, "2026/05/02/IMG_0200.MOV"))
	if !bytes.Equal(b, a) {
		t.Fatal("다시 연결 뒤 이어 받은 내용이 달라요")
	}
}

func TestInterruptedJobResumesAfterRestart(t *testing.T) {
	e := setupApp(t)
	d := time.Date(2026, 6, 1, 8, 0, 0, 0, time.Local)
	e.put(t, "IMG_0300.JPG", 1000, d)
	big := e.put(t, "IMG_0301.MOV", 3_000_000, d.Add(time.Hour))
	e.put(t, "IMG_0302.JPG", 1000, d.Add(2*time.Hour))
	os.WriteFile(filepath.Join(filepath.Dir(e.root), "..", ".rate"), []byte("2000000"), 0o644)
	e.rescan()
	if _, err := app.jobs.Create(app.lib.Assets(), app.Settings(), true, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "큰 파일 받는 중", 5*time.Second, func() bool {
		c, _ := app.jobs.State()["cur"].(map[string]any)
		return c != nil && c["name"] == "IMG_0301.MOV" && c["done"].(int64) > 200_000
	})
	app.jobs.Shutdown("앱이 꺼졌어요")

	// 앱을 다시 켠 것처럼
	app.jobs = NewJobs(e.support, e.fd, app.lib, app.journal)
	st := app.jobs.State()
	if st["stage"] != "interrupted" || st["reason"] != "앱이 꺼졌어요" {
		t.Fatalf("지난번 작업을 알아보지 못해요: %v", st)
	}
	if st["doneFiles"].(int) != 1 || st["total"].(int) != 3 || st["partial"] == nil {
		t.Fatalf("남은 수 · 받다 만 파일 정보가 이상해요: %v", st)
	}
	os.Remove(filepath.Join(filepath.Dir(e.root), "..", ".rate"))
	if err := app.jobs.Resume(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "이어서 끝", 10*time.Second, func() bool { return stage() == "done" })
	st = app.jobs.State()
	if st["ok"].(int) != 3 || st["failed"].(int) != 0 {
		t.Fatalf("이어서 한 결과가 이상해요: %v", st)
	}
	b, _ := os.ReadFile(filepath.Join(e.dest, "2026/06/01/IMG_0301.MOV"))
	if !bytes.Equal(b, big) {
		t.Fatal("이어 받은 큰 파일이 원본과 달라요")
	}
	// 확인 끝난 것을 iPhone 에서 지우기
	if err := app.jobs.DeleteVerified(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "지우기 끝", 5*time.Second, func() bool { return app.jobs.State()["kind"] == "deleted" })
	if n := app.jobs.State()["deleted"].(int); n != 3 {
		t.Fatalf("지운 수가 이상해요: %d", n)
	}
	ents, _ := os.ReadDir(e.root)
	if len(ents) != 0 {
		t.Fatalf("iPhone 에 남은 파일: %d", len(ents))
	}
}

func TestDeleteSelectedImportFirst(t *testing.T) {
	e := setupApp(t)
	d := time.Date(2026, 7, 1, 8, 0, 0, 0, time.Local)
	e.put(t, "IMG_0400.JPG", 1000, d)
	e.put(t, "IMG_0401.HEIC", 1000, d)
	e.put(t, "IMG_0401.MOV", 3000, d)
	keep := e.put(t, "IMG_0402.JPG", 1000, d)
	e.rescan()
	var ids []string
	for _, a := range app.lib.Assets() {
		if a.N != "IMG_0402.JPG" {
			ids = append(ids, a.ID)
		}
	}
	if _, err := app.jobs.Create(app.assetsFor(ids, false), app.Settings(), false, ids); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "옮긴 뒤 지우기 끝", 10*time.Second, func() bool { return stage() == "done" })
	st := app.jobs.State()
	if st["deleted"].(int) != 2 || st["kind"] != "importDelete" {
		t.Fatalf("결과가 이상해요: %v", st)
	}
	ents, _ := os.ReadDir(e.root)
	if len(ents) != 1 || ents[0].Name() != "IMG_0402.JPG" {
		t.Fatalf("고르지 않은 사진까지 지웠거나 남았어요: %v", ents)
	}
	if !fileExists(filepath.Join(e.dest, "2026/07/01/IMG_0401.MOV")) {
		t.Fatal("지우기 전에 맥에 옮기지 않았어요")
	}
	_ = keep
	// 바로 지우기
	app.jobs.Dismiss()
	e.rescan()
	a := app.lib.Assets()
	if err := app.jobs.DeleteDirect([]string{a[0].ID}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "바로 지우기", 5*time.Second, func() bool { return stage() == "done" })
	ents, _ = os.ReadDir(e.root)
	if len(ents) != 0 {
		t.Fatal("바로 지우기가 안 됐어요")
	}
}
