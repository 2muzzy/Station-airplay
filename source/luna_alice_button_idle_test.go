package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Without a live AirPlay sender the plugin must not contact Luna input at all.
// This is the idle/no-regression contract: the stock button and voice belong to
// the firmware, so polling them would only add CPU and latency.
func TestLunaAliceWatcherDoesNotPollWithoutLiveSender(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(nil)
	}))
	defer host.Close()
	api := lunaVolumeAPI(t, host)
	var session atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watchLunaAliceButton(ctx, api, &session, out)
	}()

	time.Sleep(1300 * time.Millisecond)
	mu.Lock()
	idle := requests
	mu.Unlock()
	if idle != 0 {
		t.Fatalf("watcher contacted Luna %d times without a live sender", idle)
	}

	session.Store(true)
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		got := requests
		mu.Unlock()
		if got > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not start polling after a sender appeared")
		}
		time.Sleep(20 * time.Millisecond)
	}

	session.Store(false)
	time.Sleep(time.Second) // let one in-flight long poll finish
	mu.Lock()
	before := requests
	mu.Unlock()
	time.Sleep(time.Second)
	mu.Lock()
	after := requests
	mu.Unlock()
	if after > before+1 {
		t.Fatalf("watcher kept polling after the sender left: %d -> %d", before, after)
	}
	cancel()
	<-done
}

func TestLunaAlicePressUsesKernelTimestamps(t *testing.T) {
	down := func(device string, ms int64) lunaAliceEvent {
		return lunaAliceEvent{Device: device, Kind: 1, Code: aliceButtonKey, Value: 1, TimeMS: ms}
	}
	up := func(device string, ms int64) lunaAliceEvent {
		return lunaAliceEvent{Device: device, Kind: 1, Code: aliceButtonKey, Value: 0, TimeMS: ms}
	}
	var p lunaAlicePress
	if p.consume(down("keypad", 1000)) {
		t.Fatal("press-down reported as a completed short press")
	}
	if !p.consume(up("keypad", 1060)) {
		t.Fatal("60 ms press on the same device was not a short press")
	}
	var long lunaAlicePress
	long.consume(down("keypad", 0))
	if long.consume(up("keypad", 2000)) {
		t.Fatal("2 s press was misreported as a short press")
	}
	var cross lunaAlicePress
	cross.consume(down("keypad", 1000))
	if cross.consume(up("other", 1060)) {
		t.Fatal("release from a different device was accepted")
	}
	var release lunaAlicePress
	if release.consume(up("keypad", 1000)) {
		t.Fatal("release without a press was accepted")
	}
}

func TestControlTickTracksLiveSenderForButtonPolling(t *testing.T) {
	c, receiver, _ := testControls()
	now := time.Now()
	receiver.s = receiverState{Active: true, Available: true, Client: "sender", DB: percentDB(30)}
	if err := c.tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if !c.buttonSession.Load() {
		t.Fatal("live sender did not enable button polling")
	}
	receiver.s = receiverState{Available: true, Client: "", DB: percentDB(30)}
	if err := c.tick(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.buttonSession.Load() {
		t.Fatal("missing sender kept button polling enabled")
	}
	receiver.s = receiverState{Available: true, Client: "sender", DB: percentDB(30)}
	c.buttonSession.Store(true)
	receiver.fail = true
	if err := c.tick(context.Background(), now.Add(2*time.Second)); err == nil {
		t.Fatal("expected receiver error")
	}
	if c.buttonSession.Load() {
		t.Fatal("receiver error kept button polling enabled")
	}
}
