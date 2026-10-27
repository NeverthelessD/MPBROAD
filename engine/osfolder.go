package main

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// 저장 폴더 고르기 (맥의 폴더 선택 창). 시험할 때는 MPB_TEST_CHOOSE 가 고른 결과예요.
func (a *App) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Start string `json:"start"`
	}
	readJSON(r, &in)
	if t := env("TEST_CHOOSE"); t != "" {
		writeJSON(w, map[string]any{"ok": true, "path": t})
		return
	}
	if runtime.GOOS != "darwin" {
		jsonError(w, 501, "macOS에서만 쓸 수 있어요")
		return
	}
	start := in.Start
	if !dirExists(start) {
		start, _ = os.UserHomeDir()
	}
	script := []string{
		"on run argv",
		"activate",
		"try",
		"set f to choose folder with prompt (item 2 of argv) default location (POSIX file (item 1 of argv))",
		"return POSIX path of f",
		"on error number -128",
		"return \"\"",
		"end try",
		"end run",
	}
	args := []string{}
	for _, l := range script {
		args = append(args, "-e", l)
	}
	args = append(args, start, "사진을 정리해 넣을 폴더를 골라 주세요")
	out, err := exec.Command("/usr/bin/osascript", args...).Output()
	if err != nil {
		jsonError(w, 500, "폴더 선택 창을 열지 못했어요")
		return
	}
	p := strings.TrimSpace(string(out))
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	writeJSON(w, map[string]any{"ok": true, "path": p})
}
