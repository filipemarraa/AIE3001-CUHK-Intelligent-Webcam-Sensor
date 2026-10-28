# Validation protocol

How the team measures the real heart-rate error of the camera against a reference device, and calibrates the minimum SNR for showing a heart rate. The [exhibition checklist](exhibition-checklist.md) is a separate go/no-go check of the demo on the day; this protocol produces the accuracy numbers.

Nothing here is a result yet. Recordings hold numbers only (no frames, no names) and stay under `data/`, which is gitignored.

## Reference device

1. **Chest strap with Bluetooth Heart Rate Service, preferred: Polar H10.** ECG-based, sends a heart rate about once a second and the beat-to-beat (RR) intervals. Wet the electrodes and wear it for 2 min before the session so the contact is stable.
2. **Secondary: a pulse oximeter that exports continuously over Bluetooth** with the standard Heart Rate Service (GATT service `0x180D`, characteristic `0x2A37`). Check before buying: many oximeters only show the number on their screen, or use a proprietary protocol the lab cannot read. Clip it on the hand resting on the lap, not near the face.
3. **Fallback only: a watch read by hand** and typed into the lab page. It gives one reading every 10 to 15 s, timed when typing starts. Use it for pilot sessions, not for the reported numbers.

The lab page connects to the device with Web Bluetooth (Chrome or Edge, served from `http://127.0.0.1:4301/` or https; press "Connect heart rate sensor" and pick the device). Each notification becomes one `reference` line timed with `performance.now()`, the same clock as the samples, with `source: "ble"`, the device name and the RR intervals when the device sends them. Notifications with "no skin contact" are not saved.

## What to log

The lab page writes all of these into the JSON-lines file:

| Field | Where | Note |
|---|---|---|
| Participant code | header `subject` | `P1`, `P2`… never a name |
| Condition | header `condition` | good light, low light, talking and motion, post-exercise, empty scene |
| Monk scale (1 to 10) | header `monk` | only if the participant volunteers it; otherwise "not given" |
| Camera | header `camera` | label, resolution and the frame rate the camera promised |
| Exposure lock | header `options.lockExposure` | on or off; the browser may ignore it, so note the camera model |
| Capture options | header `options` | skin region, face detection threshold, robust pixels |
| Effective frame rate | sample `timeMs` | the replay prints the samples per second (fps); frame cost and dropped frames are in the `diagnostics` lines |
| Reference | `reference` lines | heart rate, device, RR intervals |

Write by hand, in the session sheet next to the file: the light source (window, ceiling LED, ring light), anything unusual (participant moved away, strap lost contact), and the laptop and browser.

## Sessions

At least **10 participants spread over the Monk scale 1 to 10** (aim for at least three in 1 to 3, in 4 to 7 and in 8 to 10). For each participant, one recording of **2 min** per condition:

1. **Good light**: face lit from the front by diffuse light, seated still.
2. **Low light**: ceiling light only, or the front light dimmed until the "low light" reason almost appears.
3. **Talking and motion**: the participant talks naturally and moves the head as in a conversation.

And at least one participant (more if possible) with:

4. **Post-exercise**: right after 1 to 2 min of stairs or jumping jacks, so the heart rate falls during the recording. This is the session that shows lag: the reference changes by 20 bpm or more within the 2 min.

Procedure per recording:

1. Consent first. Fill participant code, condition and Monk scale (if given) on the lab page.
2. Connect the reference sensor and wait until it shows a heart rate.
3. Start, keep the participant in front of the camera for 2 min, Stop, Download. The first 10 s are warm-up and are not scored.
4. Check the reference counter grew by about one per second. If the sensor disconnected, reconnect it and repeat the recording.
5. Also record one **empty scene** of 30 s per setup: the face share in the replay must stay near zero.

## Running the replay

```bash
cd engine
go run ./cmd/validate -replay ../data/lab-P1-good-light-….jsonl,../data/lab-P2-low-light-….jsonl
```

Each estimate is compared with the **mean of the reference over the same analysis window**: the 15 s before the estimate, since the engine estimates the heart rate over its last 15 s. An estimate is scored only when that window holds at least 5 reference readings. Flags:

- `-ref-window 15`: seconds of reference averaged for each estimate (keep it equal to the engine's `HRWindowS`).
- `-min-refs 5`: fewest reference readings in the window. For a watch typed by hand every 10 to 15 s, use `-min-refs 1`.

The report has one row per recording and skin region, then all recordings pooled per skin region and SNR band:

- **evaluated**: estimates after the warm-up with enough reference in their window; **coverage**: share of them that showed a heart rate.
- **MAE**, **RMSE** and **±5 bpm** (share of shown estimates within 5 bpm of the reference).
- **bias** and **limits of agreement** (Bland-Altman: mean of estimate minus reference, plus or minus 1.96 standard deviations).
- **harmonics**: estimates within 5 bpm of double or half the reference, the typical failure of the spectral peak.
- per SNR band, **share** is the part of the evaluated estimates that fell in that band.

To choose the minimum SNR for showing a heart rate, read the pooled bands of the region in use: the cut-off is the lowest band whose ±5 bpm share and harmonics are acceptable, traded against the coverage lost below it. Change the threshold in `engine/quality` only with a test that encodes these numbers, and put the report in the pull request.

Report the results per Monk group and per condition: run the replay once per group of files.
