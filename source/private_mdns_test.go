package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateMDNSPreparesMissingAccountsWithoutChangingFirmware(t *testing.T) {
	system, state := t.TempDir(), t.TempDir()
	for name, content := range map[string]string{"passwd": "root:x:0:0:root:/root:/bin/sh\n", "group": "root:x:0:\n"} {
		if err := os.WriteFile(filepath.Join(system, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	bus, err := preparePrivateMDNS(state, system)
	if err != nil {
		t.Fatal(err)
	}
	if bus != filepath.Join(state, "private-bus") {
		t.Fatalf("bus=%q", bus)
	}
	for _, name := range []string{"passwd", "group"} {
		original, err := os.ReadFile(filepath.Join(system, name))
		if err != nil || strings.Contains(string(original), "avahi:") {
			t.Fatalf("firmware %s modified: %q, %v", name, original, err)
		}
		private, err := os.ReadFile(filepath.Join(state, name))
		if err != nil || strings.Count(string(private), "avahi:") != 1 {
			t.Fatalf("private %s=%q, %v", name, private, err)
		}
	}
	conf, err := os.ReadFile(filepath.Join(state, "bus.conf"))
	if err != nil || !strings.Contains(string(conf), "unix:path="+bus) || strings.Contains(string(conf), "/run/dbus/system_bus_socket") {
		t.Fatalf("private bus config=%q, %v", conf, err)
	}
}

func TestPrivateMDNSPreservesExistingAccounts(t *testing.T) {
	system, state := t.TempDir(), t.TempDir()
	for name, content := range map[string]string{"passwd": "avahi:x:300:400:Avahi:/run/avahi:/bin/false", "group": "avahi:x:400:"} {
		if err := os.WriteFile(filepath.Join(system, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := preparePrivateMDNS(state, system); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"passwd", "group"} {
		original, _ := os.ReadFile(filepath.Join(system, name))
		private, _ := os.ReadFile(filepath.Join(state, name))
		if string(private) != string(original) {
			t.Fatalf("existing %s replaced: %q", name, private)
		}
	}
}

func TestPrivateMDNSStopsStartupWhenBusExits(t *testing.T) {
	system, state := t.TempDir(), t.TempDir()
	for _, name := range []string{"passwd", "group"} {
		if err := os.WriteFile(filepath.Join(system, name), []byte("root:x:0:0:\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := preparePrivateMDNS(state, system); err != nil {
		t.Fatal(err)
	}
	exits := make(chan exitInfo, 1)
	launch := func(name, bin string, args ...string) error {
		exits <- exitInfo{name, errors.New("bus failed")}
		return nil
	}
	err := startPrivateMDNS(context.Background(), t.TempDir(), state, midiProfile, nil, launch, exits)
	if err == nil || !strings.Contains(err.Error(), "dbus-daemon exited during startup") {
		t.Fatalf("err=%v", err)
	}
}

func TestPrivateMDNSStopsStartupWhenAvahiExits(t *testing.T) {
	state := t.TempDir()
	// A socket alone is not a ready mDNS service. Simulate the bus becoming
	// available, then Avahi failing before acquiring its D-Bus name.
	socket, err := net.Listen("unix", filepath.Join(state, "private-bus"))
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	exits := make(chan exitInfo, 1)
	launch := func(name, bin string, args ...string) error {
		if name == "avahi-namespace" {
			exits <- exitInfo{name, errors.New("missing user")}
		}
		return nil
	}
	// No stock hostname lookup is needed for this startup-failure case.
	err = startPrivateMDNS(context.Background(), t.TempDir(), state, stationProfile{}, nil, launch, exits)
	if err == nil || !strings.Contains(err.Error(), "avahi-namespace exited during startup") {
		t.Fatalf("err=%v", err)
	}
}
