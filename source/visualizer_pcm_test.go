package main

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestVisualizerKeepsDrainingWhenDisplayDoesNotConsume(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pcm")
	fd, err := ensureFIFO(path)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	blocks := make(chan visualizerPCM, 1) // Deliberately never read display samples.
	go func() { done <- readVisualizerPCM(ctx, fd, blocks) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	written := make(chan error, 1)
	go func() { _, err := writer.Write(make([]byte, 1024*1024)); written <- err }()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("slow display blocked the audio FIFO writer")
	}
	if len(blocks) != 1 {
		t.Fatal("expected a bounded queued analysis block")
	}
}

func TestVisualizerUsesTheSameFrequencyBandsAt48And96k(t *testing.T) {
	for _, rate := range []int{48000, 96000} {
		pcm := make([]byte, rate/4*4)
		for frame := 0; frame < len(pcm)/4; frame++ {
			value := uint16(int16(12000 * math.Sin(2*math.Pi*1050*float64(frame)/float64(rate))))
			binary.LittleEndian.PutUint16(pcm[frame*4:], value)
			binary.LittleEndian.PutUint16(pcm[frame*4+2:], value)
		}
		s := &airplayState{sampleRate: rate}
		s.audio(pcm, time.Now())
		for _, band := range []int{0, 1, 3} {
			if s.history[2] <= s.history[band] {
				t.Fatalf("rate %d: 1050 Hz mapped to wrong bands: %v", rate, s.history)
			}
		}
		if s.pcmFrames != uint64(len(pcm)/4) {
			t.Fatal("PCM frame count changed")
		}
	}
}
