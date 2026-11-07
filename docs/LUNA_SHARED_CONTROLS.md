# Route shared controls through Luna and improve volume synchronization

## Summary

- Use Luna runtime endpoints for native-player pause/release and Alice-button
  events on Luna hosts; retain legacy-host backends.
- Gate button long-polling on a live sender and avoid native-player release
  requests before acquiring a possible lease.
- Keep Luna as the sole volume owner, including legacy package settings;
  do not issue an independent ALSA write after an HTTP failure.
- Avoid a redundant station read for a known sender-volume change, retaining
  guarded write/readback and handoff logic.
- Apply a 10% sender write grid and curve exponent 0.8, with inverse feedback;
  retain the station's native 5% observation grid.
- Allow the original Mini's packaged-D-Bus profile to run qualified
  volume-only controls without enabling unqualified speech/button control.
- Declare narrow media.native and input.alice permissions.

## Explicitly out of scope

No PCM spectrum, artwork cache/palette, RGB channel remapping, LED matrix,
brightness option, pause-colour change or new LED writer. Public receiver
configuration and indication logic are retained. No release/version bump,
prebuilt binaries, device data or deployment is included.

## Companion requirement / release gate

Requires the runtime API released in Luna v0.2.24. Older Luna builds do not
implement those endpoints. Capability detection remains in place; a missing
capability disables the optional handover/button with a diagnostic rather than
inventing success. The published package declares `yam/luna >=0.2.24`.

## Validation of this candidate

- go test ./...: PASS.
- go test -race -count=1 ./...: PASS.
- Bounded host cgroup runs: cleanup confirmed, peak 62 C.
- Synthetic HTTP/idle/button/volume regression tests included.
- The opt-in external HTTP fixture test is skipped without its isolated fixture.
- New candidate package, emulator lifecycle, phone playback/button/voice and
  physical-device validation: NOT_RUN; no live device changed by this PR prep.
- Earlier live experiments are not acceptance evidence for this new branch.

## Base / submission status

Released as Station-airplay 2.1.0 from v2.0.9
(3339feec989c34e1645b329446580568b5989ff2).
