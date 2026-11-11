# 2.1.3 volume contract

The 10% sender-volume step is included in 2.1.3. This is retained behaviour,
not a newly introduced change: it shipped in the 2.1.0 source and is also in
the v2.1.2 tag.

For shared controls with station-volume forwarding enabled:

- Map sender volume from -30..0 dB through exponent 0.8, then round to the
  nearest 10 station percentage points (0, 10, ..., 100).
- -144 dB means mute. Invalid receiver values are rejected before writing.
- Return station volume using the inverse curve. Native observations remain
  on their own 5% grid; observing 25% must not turn into a 30% write on handoff.
- These are 10 percentage-point write levels, not a promise that each phone
  button press changes the station by exactly 10%, or that hardware/voice
  volume is quantized to 10%.
- Profiles without station-volume forwarding keep their existing receiver
  volume handling. The 2.1.3 Luna dependency remains >=0.2.25.

Examples:

| Sender dB | Station % |
|---|---|
| -144 / -30 | 0 |
| -25 | 20 |
| -20 | 40 |
| -15 | 60 |
| -10 | 70 |
| -5 | 90 |
| 0 | 100 |

`source/volume_release_contract_test.go` pins the constants independently,
checks all eleven write levels and the monotonic curve, exercises sender
changes through the shared controller, rejects invalid receiver input without
writes, and confirms the 25% handoff does not raise the station.

The tests use mocks, not physical speakers. They do not qualify Mini 3 audio,
buttons, voice, multiroom or cold boot. Artwork colour, spectrum and device
calibration are a separate experiment and are not added to this release.
