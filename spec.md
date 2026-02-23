# ISS Terminal App Specification

## Overview

Build a self-contained terminal application in Go that:

- Animates a two-frame ASCII ISS (frame switch every 1 second).
- Fetches live ISS position from `https://api.wheretheiss.at/v1/satellites/25544`.
- Reverse geocodes coordinates via `https://nominatim.openstreetmap.org/reverse`.
- Shows a compact telemetry box under the animation: `ISS over: {country}`.
- Keeps animation and network polling independent so rendering stays smooth.

The app prioritizes clarity, minimal dependencies, and a clean terminal experience.

## Library Decision

### Primary choice

- `github.com/charmbracelet/bubbletea`

### Why this is the best fit

- Built-in tick/update model (`tea.Tick`) makes separate animation and telemetry loops straightforward.
- Async command pattern (`tea.Cmd`) keeps HTTP calls non-blocking.
- Stable rendering and resize handling without heavy widget framework overhead.
- Minimal stack for this scope.

### Optional styling helper

- `github.com/charmbracelet/lipgloss` (optional)
- For this MVP, plain ASCII rendering is acceptable and preferred for minimal dependencies.

### Alternatives considered

- `tview`: excellent for widget-heavy apps but overkill for a small animation + status box app.
- `gocui`: workable, but lower-level with more manual event/render plumbing.

## Functional Requirements

1. **Animation**
   - Use two ASCII frames from:
     - `ref/frame1.txt`
     - `ref/frame2.txt`
   - Alternate frames exactly every 1 second.

2. **Telemetry**
   - Poll ISS coordinates periodically (recommended: every 5 seconds).
   - Reverse geocode to country-level result.
   - Display `ISS over: {country}`.
   - If reverse geocode has no country or returns an error like `Unable to geocode`, display `ISS over: Ocean`.

3. **Terminal UI**
   - Top: current ASCII frame.
   - Below: small ASCII box with one telemetry line.
   - Include quit controls: `q` and `ctrl+c`.

4. **Responsiveness**
   - Animation must continue while network requests are in progress.
   - Transient network errors should not crash the app.

## Non-Functional Requirements

- Keep dependencies minimal (Bubble Tea + Go stdlib; optional Lip Gloss).
- Keep code readable and modular.
- Avoid excessive API usage.
- Maintain clean behavior on terminal resize.

## External API Contracts

### ISS Position API

- Endpoint: `GET https://api.wheretheiss.at/v1/satellites/25544`
- Required fields:
  - `latitude` (float)
  - `longitude` (float)

Rate note from provider docs:

- Roughly limited to about 1 request/second.
- Planned polling at 5 seconds is safely below that.

### Reverse Geocoding API (Nominatim)

- Endpoint pattern:
  - `GET https://nominatim.openstreetmap.org/reverse?format=jsonv2&lat={lat}&lon={lon}&zoom=3&addressdetails=1`
- Country extraction from JSON path:
  - `address.country`

Usage policy constraints to follow:

- Provide a valid identifying `User-Agent` (do not use default Go HTTP user agent).
- Keep request rate low (max 1 req/sec; this app is below that).
- Respect attribution/licensing requirements if distributed.

## Proposed Runtime Design

### Model state

- `frames []string` (loaded from the two reference files)
- `frameIndex int`
- `issOver string` (country or `Ocean`)
- `lastErr string` (optional, non-fatal)
- `width int`, `height int` (for resize-aware layout)

### Message types

- `animTickMsg`
- `telemetryTickMsg`
- `issDataMsg` (lat/lon)
- `countryMsg` (country or ocean)
- `errMsg`

### Command flow

1. `Init()` returns both:
   - animation tick command (1 second)
   - telemetry tick command (5 seconds)
2. On `animTickMsg`:
   - toggle frame index
   - schedule next animation tick
3. On `telemetryTickMsg`:
   - issue async command chain:
     - fetch ISS position
     - reverse geocode
     - emit `countryMsg`
   - schedule next telemetry tick
4. On `countryMsg`:
   - update `issOver`
5. On errors:
   - keep previous `issOver` value
   - optionally update `lastErr`

### HTTP behavior

- Use one `http.Client` with timeout (recommended: 5-10 seconds).
- Set explicit headers for Nominatim requests:
  - `User-Agent: iss-tui/1.1 (+contact-or-repo-url)`
- Decode JSON with typed structs.

## Rendering Specification

### Main layout

1. Render active frame as-is.
2. Add one blank line.
3. Render telemetry box, for example:

```text
+-------------------------+
| ISS over: United States |
+-------------------------+
```

### Fallback text

- Initial state before first fetch: `ISS over: Resolving...`
- No country found / geocode miss: `ISS over: Ocean`

## Error Handling Rules

- Never terminate app on network/JSON errors.
- Keep animation running regardless of telemetry failures.
- Retry on next telemetry tick.
- Treat missing `address.country` as ocean.

## File/Project Layout (MVP)

```text
.
|- ref/
|  |- frame1.txt
|  |- frame2.txt
|- main.go
|- go.mod
|- spec.md
```

## Acceptance Criteria

- App starts and renders frame + telemetry box.
- Frame alternates every second consistently.
- `ISS over` value updates from live API data.
- Over ocean coordinates resolve to `Ocean` fallback.
- App remains responsive during slow or failing network calls.
- `q` and `ctrl+c` quit cleanly.
