package main

import (
	"errors"
	"io"
	"os"
	"runtime"
	"time"
)

func unmount(dst string) func() error {
	return func() error { return run("umount", dst) }
}

func loadFiles(dst, src string) (cleanup func() error, err error) {
	switch runtime.GOOS {
	case "linux":
		cleanup = unmount(dst)
		err = run("mount", "--bind", "-r", src, dst)
	case "darwin":
		cleanup = unmount(dst)
		err = run("bindfs", "--perms=a-w", src, dst)
	// TODO: nullfs on BSD?
	default:
		return nil, errors.New(runtime.GOOS + " not supported yet")
	}

	if err != nil {
		return nil, err
	}

	// Mounting takes a while, and I'm not sure of a decent cross-platform way
	// of determining when dst is mounted.
	const (
		start = 500 * time.Millisecond
		max   = 10 * time.Second
	)
	var t *time.Timer
	for backoff := start; backoff < max; backoff *= 2 {
		switch err := checkMount(dst); err {
		case nil:
			return cleanup, nil
		case io.EOF:
			if t == nil {
				t = time.NewTimer(backoff)
			} else {
				t.Reset(backoff)
			}
			<-t.C
		default:
			return nil, err
		}
	}
	return nil, errors.New("mount took too long")
}

func checkMount(dst string) error {
	dir, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer dir.Close()
	_, err = dir.Readdir(1) // Readdirnames calls Readdir.
	return err
}
