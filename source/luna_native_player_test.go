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

func TestLunaNativePlayerUsesTargetedRuntimeAPI(t *testing.T) {
	var mu sync.Mutex
	var actions []string
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("X-Luna-Token") != "synthetic-token" {
			t.Error("missing native runtime authentication")
		}
		switch r.URL.Path {
		case "/api/runtime/fixture/native-player/pause", "/api/runtime/fixture/native-player/release":
		default:
			t.Errorf("native operation went to wrong endpoint: %s", r.URL.Path)
		}
		mu.Lock()
		actions = append(actions, r.URL.Path)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"submitted": true})
	}))
	defer host.Close()
	n := newLunaNativePlayer(lunaVolumeAPI(t, host))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := n.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	n.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 2 {
		t.Fatalf("expected explicit pause and release, got %v", actions)
	}
}
func TestLunaNativePlayerRejectsUnacceptedSubmission(t *testing.T) {
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]bool{"submitted": false})
	}))
	defer host.Close()
	n := newLunaNativePlayer(lunaVolumeAPI(t, host))
	defer n.Close()
	if err := n.Pause(context.Background()); err == nil {
		t.Fatal("unaccepted native pause reported success")
	}
}
