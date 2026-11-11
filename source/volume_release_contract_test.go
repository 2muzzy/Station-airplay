package main

import (
	"context"
	"math"
	"testing"
	"time"
)

// Pin the user-visible release contract independently of the implementation
// constants: changing both the constant and a constant-driven test must fail.
func TestReleaseVolumeContract(t *testing.T) {
	if volumeStepPercent != 10 || volumeCurveExponent != 0.8 {
		t.Fatal("release requires the 10-percent sender grid and 0.8 curve")
	}
	seen := make(map[int]bool)
	previous := -1
	for i := 0; i <= 3000; i++ {
		db := -30 + float64(i)/100
		got := dbPercent(db)
		if got < previous || got < 0 || got > 100 || got%10 != 0 {
			t.Fatalf("invalid sender mapping at %g dB: %d after %d", db, got, previous)
		}
		seen[got] = true
		previous = got
	}
	for percent := 0; percent <= 100; percent += 10 {
		if !seen[percent] || dbPercent(percentDB(percent)) != percent {
			t.Fatalf("unreachable or unstable sender step %d", percent)
		}
	}
	for percent := 5; percent <= 95; percent += 10 {
		got, err := nativePercent(float64(percent))
		if err != nil || got != percent {
			t.Fatalf("native %d must not snap to sender grid: %d, %v", percent, got, err)
		}
	}
}

func TestReleaseSenderCurveReachesStation(t *testing.T) {
	for _, tc := range []struct {
		db   float64
		want int
	}{
		{-144, 0}, {-30, 0}, {-25, 20}, {-20, 40},
		{-15, 60}, {-10, 70}, {-5, 90}, {0, 100},
	} {
		c, receiver, station := testControls()
		ctx, now := context.Background(), time.Now()
		if err := c.tick(ctx, now); err != nil {
			t.Fatal(err)
		}
		receiver.s.DB = tc.db
		if err := c.tick(ctx, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if len(station.sets) != 1 || station.sets[0] != tc.want || station.v != float64(tc.want) {
			t.Fatalf("sender %g dB: writes %v, station %g; want %d", tc.db, station.sets, station.v, tc.want)
		}
	}
}

func TestReleaseInvalidSenderVolumeNeverWrites(t *testing.T) {
	for _, db := range []float64{-31, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		c, receiver, station := testControls()
		ctx, now := context.Background(), time.Now()
		if err := c.tick(ctx, now); err != nil {
			t.Fatal(err)
		}
		receiver.s.DB = db
		if err := c.tick(ctx, now.Add(time.Second)); err == nil {
			t.Fatalf("accepted invalid sender volume %g", db)
		}
		if len(station.sets) != 0 || station.v != 70 {
			t.Fatalf("invalid sender changed volume: %v, %g", station.sets, station.v)
		}
	}
}

func TestReleaseNativeHandoffDoesNotRaiseFivePercentLevel(t *testing.T) {
	c, receiver, station := testControls()
	station.v = 25
	receiver.s.DB = -10
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(station.sets) != 0 || station.v != 25 || len(receiver.volumes) != 1 {
		t.Fatalf("handoff rewrote native volume: %v, %g, %v", station.sets, station.v, receiver.volumes)
	}
	want := 30*math.Pow(0.25, 1.25) - 30
	if math.Abs(receiver.volumes[0]-want) > 1e-9 {
		t.Fatalf("feedback %g, want inverse curve %g", receiver.volumes[0], want)
	}
	// The sender acknowledges our own feedback; this must not snap 25 to 30.
	receiver.s.DB = receiver.volumes[0]
	for tick := 1; tick <= 4; tick++ {
		if err := c.tick(ctx, now.Add(time.Duration(tick)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if len(station.sets) != 0 || station.v != 25 {
		t.Fatalf("feedback changed station: %v, %g", station.sets, station.v)
	}
}
