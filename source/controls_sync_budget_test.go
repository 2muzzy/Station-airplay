package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type budgetStation struct {
	*mockStation
	reads      int
	writes     int
	readError  error
	writeError error
}

func (s *budgetStation) GetVolume(ctx context.Context) (float64, error) {
	s.reads++
	if s.readError != nil {
		return 0, s.readError
	}
	return s.mockStation.GetVolume(ctx)
}
func (s *budgetStation) SetVolume(ctx context.Context, value int) (float64, error) {
	s.writes++
	if s.writeError != nil {
		return 0, s.writeError
	}
	return s.mockStation.SetVolume(ctx, value)
}

func TestReadySenderChangeSkipsOnlyRedundantPreRead(t *testing.T) {
	c, r, native := testControls()
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	s := &budgetStation{mockStation: native, readError: errors.New("pre-read must not run")}
	c.station = s
	r.s.DB = percentDB(40)
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.reads != 0 || s.writes != 1 || c.lastNative != 40 || native.v != 40 {
		t.Fatalf("reads=%d writes=%d native=%v", s.reads, s.writes, native.v)
	}
}

func TestUnchangedSenderStillReadsNativeVolume(t *testing.T) {
	c, _, native := testControls()
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	s := &budgetStation{mockStation: native}
	c.station = s
	if err := c.tick(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if s.reads != 1 || s.writes != 0 {
		t.Fatalf("reads=%d writes=%d", s.reads, s.writes)
	}
}

func TestSenderWriteFailureDoesNotCommitVolume(t *testing.T) {
	c, r, native := testControls()
	ctx, now := context.Background(), time.Now()
	if err := c.tick(ctx, now); err != nil {
		t.Fatal(err)
	}
	oldDB, oldNative := c.lastDB, c.lastNative
	s := &budgetStation{mockStation: native, writeError: errors.New("readback mismatch")}
	c.station = s
	r.s.DB = percentDB(40)
	if err := c.tick(ctx, now.Add(time.Second)); err == nil {
		t.Fatal("write failure ignored")
	}
	if c.lastDB != oldDB || c.lastNative != oldNative || c.volumeReady || native.v != 70 {
		t.Fatal("failed write committed state")
	}
}

func TestInitialHandoffStillReadsNativeFirst(t *testing.T) {
	c, r, native := testControls()
	s := &budgetStation{mockStation: native}
	c.station = s
	r.s.DB = percentDB(20)
	if err := c.tick(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if s.reads != 1 || s.writes != 0 || len(r.volumes) != 1 {
		t.Fatal("initial handoff changed")
	}
}
