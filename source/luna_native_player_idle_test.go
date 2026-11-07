package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLunaNativePlayerIdleNeverContactsRuntime(t *testing.T) {
	var mu sync.Mutex
	var actions []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actions = append(actions, r.URL.Path)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"submitted": true})
	}))
	defer host.Close()
	n := newLunaNativePlayer(lunaVolumeAPI(t, host))
	// The controller calls Stop every idle tick, including before any AirPlay
	// session. Neither those calls nor shutdown should touch the stock player.
	for i := 0; i < 20; i++ {
		n.Stop()
	}
	n.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 0 {
		t.Fatalf("idle adapter contacted stock-player runtime: %v", actions)
	}
}

func TestLunaNativePlayerIdleControllerDoesNotInterfere(t *testing.T) {
	var mu sync.Mutex
	var actions []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actions = append(actions, r.URL.Path)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"submitted": true})
	}))
	defer host.Close()
	n := newLunaNativePlayer(lunaVolumeAPI(t, host))
	c, receiver, station := testControls()
	c.cfg.NativePlayer, c.cfg.AliceButton = true, true
	c.native = n
	receiver.s = receiverState{DB: percentDB(30)}
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 20; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		if err := c.tick(ctx, at); err != nil {
			t.Fatal(err)
		}
		if err := c.button(ctx, at); err != nil {
			t.Fatal(err)
		}
		if err := c.speech(ctx, speechEvent{Phase: "recognition", RequestID: "stock", Final: true, Transcript: "включи музыку"}, at); err != nil {
			t.Fatal(err)
		}
	}
	c.close(ctx)
	n.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 0 || len(receiver.commands) != 0 || len(station.sets) != 0 || len(station.frames) != 0 || station.off != 0 {
		t.Fatalf("idle controller touched player/transport/volume/LEDs: requests=%v receiver=%v sets=%v frames=%v off=%d", actions, receiver.commands, station.sets, station.frames, station.off)
	}
}

func TestLunaNativePlayerStopReleasesOnlyAnOwnedSession(t *testing.T) {
	var mu sync.Mutex
	var actions []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actions = append(actions, r.URL.Path)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"submitted": true})
	}))
	defer host.Close()
	n := newLunaNativePlayer(lunaVolumeAPI(t, host))
	if err := n.Pause(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A synchronous release is a queue barrier: it guarantees prior work has
	// completed without relying on a timing sleep in this test.
	reply := make(chan error, 1)
	n.tasks <- lunaNativeTask{action: "release", ctx: context.Background(), reply: reply}
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	n.Stop()
	n.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 2 || actions[0] != "/api/runtime/fixture/native-player/pause" || actions[1] != "/api/runtime/fixture/native-player/release" {
		t.Fatalf("expected exactly one pause and one release, got %v", actions)
	}
}
