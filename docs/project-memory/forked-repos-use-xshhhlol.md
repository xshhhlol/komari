---
name: forked-repos-use-xshhhlol
description: "komari + its frontend (komari-web) and agent (komari-agent) all use the user's xshhhlol forks, never the komari-monitor originals"
metadata: 
  node_type: memory
  type: project
  originSessionId: 78da490c-3944-4862-befe-c849c8087cf7
---

This project is one of a **3-repo system**, and the user runs their **own forks** under the `xshhhlol` GitHub org. Always target these forks — never `komari-monitor/*` (the upstream originals).

| Role | Local path | Remote (use this) | Default branch | Upstream (do NOT use) |
| --- | --- | --- | --- | --- |
| Backend (this repo) | `/Users/asche/GolandProjects/komari` | `https://github.com/xshhhlol/komari` | `main` | komari-monitor/komari |
| Frontend | `/Users/asche/GolandProjects/komari-web` | `https://github.com/xshhhlol/komari-web` | `radix` | komari-monitor/komari-web |
| Agent (runs on monitored hosts) | `/Users/asche/GolandProjects/komari-agent` | `https://github.com/xshhhlol/komari-agent` | `main` | komari-monitor/komari-agent |

Build / runtime relationships:
- The frontend is **embedded into the backend binary** at build time: backend CI (`.github/workflows/*.yml`) clones `xshhhlol/komari-web`, runs `npm run build`, copies into `web/public/defaultTheme/dist`, then `//go:embed`. A frontend change therefore only ships after a new **backend** release + redeploy.
- The agent **auto-updates** from `xshhhlol/komari-agent` (`update.Repo` constant in the agent) every 6h + on startup; `install.sh`/`install.ps1` and the panel's generated install command also point at the agent fork.
- The Go module path stays `github.com/komari-monitor/komari-agent` (and `.../komari`) — that is the import path, do NOT change it. Only repo/clone/download/release URLs were repointed to `xshhhlol`.

**Why:** The user forked all three to ship their own fixes — e.g. the web-terminal vim fixes (see [[webterm-vim-freeze-root-cause]]: vite build target es2022 is the REAL fix; plus XTGETTCAP DCS 0+r responder, right-click paste via `term.paste()`, agent `TERM=xterm-256color` with `KOMARI_AGENT_TERM` env escape hatch), `onBinary` forwarding in the frontend, and terminal-relay robustness (keepalive/recover/race fixes) in the backend. Pointing at `komari-monitor` would lose those, and the agent's auto-update would actively overwrite the forked agent with the official build.

Release/deploy flow (established 2026-07-08): ship via GitHub releases — `gh release create <tag> -R xshhhlol/<repo> --target <default branch>`; both repos' CI triggers on `release: published`. Backend release CI clones komari-web@radix fresh, so frontend changes ship by just cutting a new backend release (no backend commit needed; `web/public/defaultTheme/dist` is gitignored). Agent version = release tag via ldflags (`update.CurrentVersion`); agents auto-update from latest release within ~6h. The panel binary is updated manually by the user. Version pattern: backend minor bumps (2.19.0→2.20.0), agent patch/minor as appropriate.

**How to apply:** For git pushes, CI clone URLs, install scripts, release/version-update checks, and agent auto-update — use the `xshhhlol` remotes above. If you see a `komari-monitor/*` reference in any of these three repos (outside the Go module/import path), treat it as a leftover to repoint to `xshhhlol`.
