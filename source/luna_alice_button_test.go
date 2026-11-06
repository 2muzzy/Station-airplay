package main

import "testing"

func TestLunaAlicePressUsesEventTimestamps(t *testing.T) {
	for _, test := range []struct {
		ms   int64
		want bool
	}{{0, false}, {29, false}, {30, true}, {200, true}, {600, true}, {601, false}, {3000, false}, {-1, false}} {
		var p lunaAlicePress
		down := lunaAliceEvent{Device: "keypad", Kind: 1, Code: aliceButtonKey, Value: 1, TimeMS: 10000}
		up := down
		up.Value = 0
		up.TimeMS += test.ms
		if p.consume(down) {
			t.Fatal("press fired without release")
		}
		if got := p.consume(up); got != test.want {
			t.Fatalf("duration %d got %v", test.ms, got)
		}
		if p.consume(up) {
			t.Fatal("duplicate release fired")
		}
	}
}
func TestLunaAlicePressRejectsMixedDevicesAndOtherKeys(t *testing.T) {
	var p lunaAlicePress
	p.consume(lunaAliceEvent{Device: "a", Kind: 1, Code: aliceButtonKey, Value: 1, TimeMS: 100})
	if p.consume(lunaAliceEvent{Device: "a", Kind: 1, Code: 115, Value: 0, TimeMS: 200}) {
		t.Fatal("volume key fired Alice shortcut")
	}
	if p.consume(lunaAliceEvent{Device: "b", Kind: 1, Code: aliceButtonKey, Value: 0, TimeMS: 300}) {
		t.Fatal("mixed devices fired shortcut")
	}
}
