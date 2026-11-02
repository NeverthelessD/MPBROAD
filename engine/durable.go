package main

import (
	"os"
	"syscall"
)

// makeDurable: 파일들이 디스크에 확실히 내려가 있게 해요.
// 파일마다 fsync(가벼움, 이미 내려간 파일은 금방 끝나요) → 마지막에 한 번 F_FULLFSYNC(맥: 디스크 안의 캐시까지 비움).
func makeDurable(paths []string) error {
	var last *os.File
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			if last != nil {
				last.Close()
			}
			return err
		}
		if err := syscall.Fsync(int(f.Fd())); err != nil {
			f.Close()
			if last != nil {
				last.Close()
			}
			return err
		}
		if last != nil {
			last.Close()
		}
		last = f
	}
	if last != nil {
		defer last.Close()
		return last.Sync() // macOS 에서 Go 의 Sync 는 F_FULLFSYNC
	}
	return nil
}
