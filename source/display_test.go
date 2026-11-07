package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMidiSharedDisplayKeepsItsGeometryAndYieldsToAlice(t *testing.T) {
	withProfile(t, midiProfile)
	events := make(chan string, 128)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := r.URL.Path
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.URL.Path == "/led" {
			var colors []string
			_ = json.Unmarshal(body["colors"], &colors)
			if len(colors) != 24 {
				t.Errorf("Midi renderer lost its 24 LEDs: %d", len(colors))
			}
		}
		if r.URL.Path == "/clock" {
			if string(body["off"]) == "true" {
				kind = "clock-off"
			} else {
				kind = "clock-frame"
				var segments map[string]uint8
				if raw, ok := body["segments"]; ok {
					_ = json.Unmarshal(raw, &segments)
					for digit := range segments {
						if len(digit) != 2 || digit[1] < '1' || digit[1] > '4' {
							t.Errorf("wrong Midi segment %q", digit)
						}
					}
				}
			}
		}
		select {
		case events <- kind:
		default:
			t.Error("display request flood")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	api := &yamAPI{base: server.URL, tokenHeader: "Authorization", token: "test", client: server.Client()}
	meta, err := ensureFIFO(filepath.Join(t.TempDir(), "metadata"))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(meta)
	pcm := make(chan visualizerPCM, 4)
	updates := make(chan rendererControlState, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- renderDisplay(ctx, api, meta, nil, displayInput{profile: midiProfile, controls: &controlsConfig{LEDs: true}, pcm: pcm, updates: updates})
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	wait := func(want string) {
		t.Helper()
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		for {
			select {
			case got := <-events:
				if got == want {
					return
				}
			case <-timer.C:
				t.Fatalf("display did not emit %s", want)
			}
		}
	}
	update := rendererControlState{Session: true, Paint: true, Expires: time.Now().Add(time.Minute)}
	updates <- update
	pcm <- visualizerPCM{data: make([]byte, 4096), at: time.Now()}
	wait("/led")
	wait("clock-frame")
	update.Paint = false
	updates <- update
	wait("/led/off")
	wait("clock-off")
	select {
	case got := <-events:
		if got == "/led" || got == "clock-frame" {
			t.Fatalf("renderer overwrites Alice: %s", got)
		}
	case <-time.After(100 * time.Millisecond):
	}
	update.Paint = true
	updates <- update
	wait("/led")
	wait("clock-frame")
	update.Session = false
	updates <- update
	wait("/led/off")
	wait("clock-off")
}
