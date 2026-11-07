package main

import (
	"context"
	"sync/atomic"
	"time"
)

type lunaAliceEvent struct {
	Device string `json:"device"`
	Kind   int    `json:"kind"`
	Code   int    `json:"code"`
	Value  int    `json:"value"`
	TimeMS int64  `json:"time_ms"`
}
type lunaAlicePress struct {
	device  string
	pressed bool
	at      int64
}

func (p *lunaAlicePress) consume(e lunaAliceEvent) bool {
	if e.Kind != 1 || e.Code != aliceButtonKey {
		return false
	}
	switch e.Value {
	case 1:
		if !p.pressed {
			p.pressed = true
			p.at = e.TimeMS
			p.device = e.Device
		}
	case 0:
		if !p.pressed {
			return false
		}
		held := e.TimeMS - p.at
		device := p.device
		*p = lunaAlicePress{}
		// Use kernel event timestamps, not HTTP arrival times. Press and release
		// can be delivered by different polls or delayed together by the network.
		return e.Device == device && held >= aliceButtonShortMin.Milliseconds() && held <= aliceButtonShortMax.Milliseconds()
	}
	return false
}
func watchLunaAliceButton(ctx context.Context, api *yamAPI, session *atomic.Bool, out chan<- struct{}) {
	client := *api.client
	client.Timeout = 3 * time.Second
	runtime := *api
	runtime.client = &client
	var press lunaAlicePress
	for ctx.Err() == nil {
		if !session.Load() {
			// No AirPlay sender means the stock firmware owns the button. Do not
			// request Luna input at all: idle must not read manifests or compete
			// with the vendor stack for a key this plugin will ignore.
			press = lunaAlicePress{}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		var event *lunaAliceEvent
		err := runtime.requestContext(ctx, "POST", "/input/alice/next", map[string]any{}, &event)
		if err != nil {
			press = lunaAlicePress{}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		if event != nil && press.consume(*event) {
			select {
			case out <- struct{}{}:
			default:
			}
		}
	}
}
