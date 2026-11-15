---
name: headless-chrome-screenshot-hang
description: "On this Mac, headless Chrome writes --screenshot but never exits, and enforces a ~500px min window width — use background launch + poll + kill, and fake narrow widths via a fixed-width container"
metadata: 
  node_type: memory
  type: project
  originSessionId: cc46b4a4-abf0-439f-82f1-9e47c501e3fb
  modified: 2026-09-15T08:57:59.271Z
---

Visual checks of theme/UI changes (e.g. [[purcarte-theme-build-deploy]]) use `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome --headless=new --screenshot=...` against a scratchpad HTML that links the built `dist/assets/index-*.css` and wraps markup in `<div class="radix-themes light|dark" data-accent-color="...">`.

Two quirks observed 2026-09-12:
- Chrome writes the PNG, then **hangs forever** (a foreground call hit the 90s tool timeout). Launch it with `&`, poll until the file is non-empty (`[ -s "$OUT" ]`, ~1s steps, max ~40s), then `kill` the PID and `pkill -f <user-data-dir>`.
- `--window-size=400,...` does **not** give a 400px layout viewport (the window has a minimum width, the page lays out wider and gets cropped). To test phone width, set a fixed width on the content wrapper instead (e.g. `width: 344px` ≈ the mobile popup's content width).
- Give each parallel run its own `--user-data-dir` in the scratchpad, or the instances block each other.

- **Interactive pages (need clicks, e.g. PurCarte instance page → 延迟 → 1天)**: launch Chrome in the background with `--remote-debugging-port=9333`, then drive it from a Node 24 script via CDP over the built-in `WebSocket` (`/json/list` → `Runtime.evaluate` to click, `Emulation.setDeviceMetricsOverride` for width — this DOES give a true 400px layout, `Page.captureScreenshot` with `clip`+`captureBeyondViewport`). Pair it with a tiny Node mock backend serving `dist/` + REST `/api/*` (a failing `/api/version` keeps the theme off RPC). Worked end to end 2026-09-15.
- **`captureBeyondViewport: true` produces false "broken" shots** on PurCarte (chart half-cut with footer/background showing, or chart area blank on mobile) — the capture resizes/reflows the page. Instead set the emulated viewport tall enough (e.g. height 2200), `scrollIntoView()` the section, and capture with a plain `clip` (no captureBeyondViewport). Confirmed 2026-09-15.
- **Local proxy trap**: the shell profile exports `http_proxy`/`all_proxy` (127.0.0.1:1080), so `curl`/Node to `127.0.0.1` get **HTTP 502 Bad Gateway** (curl exit 22). Run localhost checks under `env -u http_proxy -u https_proxy -u all_proxy -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY NO_PROXY='*'`.

**Why:** cost two 90s timeouts plus a misleading "overflowing" mobile screenshot before the causes were clear.

**How to apply:** use the background+poll+kill pattern from the start for any headless Chrome screenshot here, and simulate narrow widths with a container width rather than the window size.
