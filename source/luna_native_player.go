package main

// Station-side player access belongs to Luna. This adapter never runs q and
// never sends a provider-broker command that could target AirPlay itself.
import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type lunaNativeTask struct {
	action string
	ctx    context.Context
	reply  chan error
}
type lunaNativePlayer struct {
	api    *yamAPI
	ctx    context.Context
	cancel context.CancelFunc
	tasks  chan lunaNativeTask
	done   chan struct{}
	once   sync.Once
	leased atomic.Bool
}

func newLunaNativePlayer(api *yamAPI) *lunaNativePlayer {
	copyAPI := *api
	client := *api.client
	client.Timeout = 12 * time.Second
	copyAPI.client = &client
	ctx, cancel := context.WithCancel(context.Background())
	n := &lunaNativePlayer{api: &copyAPI, ctx: ctx, cancel: cancel, tasks: make(chan lunaNativeTask, 1), done: make(chan struct{})}
	go n.run()
	return n
}
func (n *lunaNativePlayer) send(ctx context.Context, action string) error {
	var result struct {
		Submitted bool `json:"submitted"`
	}
	if err := n.api.requestContext(ctx, "POST", "/native-player/"+action, map[string]any{}, &result); err != nil {
		return err
	}
	if !result.Submitted {
		return errors.New("Luna did not accept native-player operation")
	}
	return nil
}
func (n *lunaNativePlayer) run() {
	defer close(n.done)
	for {
		select {
		case <-n.ctx.Done():
			return
		case task := <-n.tasks:
			if task.ctx.Err() != nil {
				if task.reply != nil {
					task.reply <- task.ctx.Err()
				}
				continue
			}
			// Match the legacy backend: Stop before any pause is strictly
			// local. Idle controller ticks must not call Luna's player API.
			if task.action == "release" && !n.leased.Load() {
				if task.reply != nil {
					task.reply <- nil
				}
				continue
			}
			// A timed-out request may already have reached Luna. Remember the
			// possible lease before sending so shutdown still releases it.
			if task.action == "pause" {
				n.leased.Store(true)
			}
			ctx, cancel := context.WithTimeout(task.ctx, 12*time.Second)
			stop := context.AfterFunc(n.ctx, cancel)
			err := n.send(ctx, task.action)
			if task.action == "release" && err == nil {
				n.leased.Store(false)
			}
			stop()
			cancel()
			if task.reply != nil {
				task.reply <- err
			}
		}
	}
}
func (n *lunaNativePlayer) Pause(ctx context.Context) error {
	reply := make(chan error, 1)
	select {
	case <-n.ctx.Done():
		return errors.New("Luna native-player adapter closed")
	case <-ctx.Done():
		return ctx.Err()
	case n.tasks <- lunaNativeTask{action: "pause", ctx: ctx, reply: reply}:
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-n.ctx.Done():
		return errors.New("Luna native-player adapter closed")
	}
}
func (n *lunaNativePlayer) PauseNow() error {
	select {
	case <-n.ctx.Done():
		return errors.New("Luna native-player adapter closed")
	default:
	}
	select {
	case n.tasks <- lunaNativeTask{action: "pause", ctx: n.ctx}:
	default: // Coalesce refreshes; never stall sender/volume synchronization.
	}
	return nil
}
func (n *lunaNativePlayer) Stop() {
	select {
	case n.tasks <- lunaNativeTask{action: "release", ctx: n.ctx}:
	default: // Luna also expires an idle native-player lease automatically.
	}
}
func (n *lunaNativePlayer) Close() {
	n.once.Do(func() {
		n.cancel()
		<-n.done
		if !n.leased.Load() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = n.send(ctx, "release")
	})
}
