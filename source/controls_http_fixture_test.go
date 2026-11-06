package main

import (
	"context"
	"os"
	"testing"
	"time"
)

type countingHTTPStation struct {
	yamControlStation
	reads int
}

func (s *countingHTTPStation) GetVolume(ctx context.Context) (float64, error) {
	s.reads++
	return s.yamControlStation.GetVolume(ctx)
}

func TestSyncBudgetHTTPFixture(t *testing.T) {
	url := os.Getenv("SYNC_BUDGET_FIXTURE_URL")
	if url == "" {
		t.Skip("isolated HTTP fixture only")
	}
	if url != "http://127.0.0.1:18099" || os.Getenv("SYNC_BUDGET_FIXTURE_DIR") != "/fixture" {
		t.Fatal("refuse non-fixture endpoint")
	}
	clearRuntimeEnvironment(t)
	t.Setenv("LUNA_API_URL", url+"/api/runtime/fixture")
	t.Setenv("LUNA_API_TOKEN", "synthetic-fixture-token")
	api, err := newYAMAPI()
	if err != nil {
		t.Fatal(err)
	}
	volume, err := newLunaControlVolume(api)
	if err != nil {
		t.Fatal(err)
	}
	c, r, _ := testControls()
	c.cfg = controlsConfig{NativeVolume: true}
	station := &countingHTTPStation{yamControlStation: yamControlStation{api: api, volume: volume}}
	c.station = station
	step := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := c.tick(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	step() // Initial handoff must still start from the station's real value.
	if len(r.volumes) != 1 {
		t.Fatal("missing initial handoff")
	}
	r.s.DB = r.volumes[0]
	step() // Sender acknowledges the station value.
	for _, want := range []int{20, 60} {
		before := station.reads
		r.s.DB = percentDB(want)
		started := time.Now()
		step()
		if station.reads != before {
			t.Fatal("redundant controller pre-read before sender write")
		}
		if c.lastNative != want {
			t.Fatalf("committed %d, want %d", c.lastNative, want)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		got, err := volume.GetVolume(ctx)
		cancel()
		if err != nil || got != float64(want) {
			t.Fatalf("independent readback %v: %v", got, err)
		}
		t.Logf("APPLIED want=%d actual=%.0f elapsed=%s", want, got, time.Since(started))
	}
	// An ignored native write must fail the unchanged adapter readback guard.
	if err := os.WriteFile("/fixture/ignore-writes", []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("/fixture/ignore-writes")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if got, err := volume.SetVolume(ctx, 90); err == nil {
		t.Fatalf("ignored write accepted: %v", got)
	}
	t.Log("IGNORED_WRITE_REJECTED_BY_READBACK")
}
