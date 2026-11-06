package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPipeChildGetsEOFBeforeTermination(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "closed-cleanly")
	cmd := exec.Command("sh", "-c", `cat >/dev/null; printf drained > "$1"`, "sh", marker)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	c := &child{name: "pipe-player", cmd: cmd, stopInput: func() { _ = in.Close() }, done: done}
	stopChildren([]*child{c})
	<-done
	if waitErr != nil {
		t.Fatalf("player was interrupted instead of draining EOF: %v", waitErr)
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "drained" {
		t.Fatalf("clean close marker=%q, err=%v", b, err)
	}
}
