package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type displayInput struct {
	profile       stationProfile
	controls      *controlsConfig
	pcm           <-chan visualizerPCM
	pcmErrors     <-chan error
	updates       <-chan rendererControlState
	controlErrors <-chan error
}

// Light 2 retains its display mode; Midi yields the same visualizer to Alice
// outside actual playback and while the shared controller reports speech.
func renderDisplay(ctx context.Context, api *yamAPI, metaFD int, exits <-chan exitInfo, input displayInput) error {
	p, controls := input.profile, input.controls
	defer func() {
		_ = api.ledOff()
		_ = api.clockOff()
	}()
	state := &airplayState{status: "stopped", sampleRate: profileOutputRate(p)}
	var controlUI rendererControlState
	released := false
	tz := clockZone()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	lastUIError := time.Time{}
	lastClockError := time.Time{}
	oldRing := ""
	oldFrameKey := ""
	lastRingSent, lastClockSent := time.Time{}, time.Time{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ex := <-exits:
			return &childFailure{ex.name, ex.err}
		case err := <-input.controlErrors:
			return fmt.Errorf("display controls stopped: %w", err)
		case err := <-input.pcmErrors:
			return fmt.Errorf("audio FIFO: %w", err)
		case update := <-input.updates:
			controlUI = update
			if update.Session && update.VolumeKnown {
				state.showVolume(update.Volume, time.Now())
			}
		case block := <-input.pcm:
			state.audio(block.data, block.at)
		case now := <-tick.C:
			if e := readFD(metaFD, func(buf []byte) {
				state.meta.feed(buf, func(t, c string, raw []byte) { state.onMetadata(t, c, raw, now) })
			}); e != nil {
				return fmt.Errorf("metadata FIFO: %w", e)
			}
			state.rotate()
			if isMidiProfile(p) {
				paint := state.pcmUIActive(now)
				if controls != nil {
					paint = paint && controls.LEDs && controlUI.allowsPaint(now)
				}
				if !paint {
					if !released {
						ringErr, clockErr := api.ledOff(), api.clockOff()
						logAPIError("ring", ringErr, &lastUIError)
						logAPIError("clock", clockErr, &lastClockError)
						released = ringErr == nil && clockErr == nil
						oldRing, oldFrameKey = "", ""
					}
					continue
				}
				released = false
			}
			colors := state.ringFrame(now)
			ringKey := strings.Join(colors, ",")
			if ringKey != oldRing || now.Sub(lastRingSent) > 3*time.Second {
				e := api.leds(colors)
				logAPIError("ring", e, &lastUIError)
				if e == nil {
					oldRing = ringKey
					lastRingSent = now
				}
			}
			timeValue, frame := state.display(now, tz)
			var key string
			if frame == nil {
				key = "time:" + timeValue
			} else {
				b, _ := json.Marshal(frame)
				key = "seg:" + string(b)
			}
			if key != oldFrameKey || now.Sub(lastClockSent) >= 1200*time.Millisecond {
				var err error
				if frame == nil {
					err = api.time(timeValue)
				} else {
					err = api.segments(frame)
				}
				logAPIError("clock", err, &lastClockError)
				if err == nil {
					oldFrameKey = key
					lastClockSent = now
				}
			}
		}
	}
}
