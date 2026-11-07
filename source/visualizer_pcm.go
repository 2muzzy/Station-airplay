package main

import (
	"context"
	"time"
)

type visualizerPCM struct {
	data []byte
	at   time.Time
}

// The supervisor keeps this reader alive through child shutdown, so a file
// tee cannot block Shairport while it is closing its shared ALSA client.
func startVisualizerPCM(fd int) (<-chan visualizerPCM, <-chan error, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	blocks := make(chan visualizerPCM, 4)
	errors := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := readVisualizerPCM(ctx, fd, blocks); err != nil {
			errors <- err
		}
	}()
	return blocks, errors, func() { cancel(); <-done }
}

// Keep draining ALSA's analysis copy even while a display request is slow.
// Display frames may be dropped; they must never back up the audio output.
func readVisualizerPCM(ctx context.Context, fd int, blocks chan<- visualizerPCM) error {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-tick.C:
			if err := readFD(fd, func(pcm []byte) {
				block := visualizerPCM{data: append([]byte(nil), pcm...), at: now}
				select {
				case blocks <- block:
				default:
				}
			}); err != nil {
				return err
			}
		}
	}
}
