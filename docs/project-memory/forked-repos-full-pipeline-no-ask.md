---
name: forked-repos-full-pipeline-no-ask
description: "For any change to the 3 xshhhlol forks, auto-run the full pipeline (verify → commit → push) without stopping to ask between steps"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 9fa7177f-b903-4971-b488-13651908ef02
---

For **any** code change to one of the three `xshhhlol` forks (komari / komari-web / komari-agent), carry it through the **full delivery pipeline automatically**, without pausing to ask between steps:

1. **Verify** as applicable to the repo (frontend: `tsc`/`eslint`/`npm run build`; backend: `go build`/`go test`).
2. **Commit** — stage only the feature-relevant files; revert incidental churn (e.g. `package-lock.json` peer/platform metadata, gitignored `dist/`). Commit directly to the fork's default branch (komari `main`, komari-web `radix`, komari-agent `main`).
3. **Push** to the matching `xshhhlol` remote.
4. **Publish** the appropriate GitHub Release automatically; do not wait for CI/build results. User explicitly confirmed this standing instruction on 2026-11-15. Release publication does not itself authorize upgrading a live server.

Do **not** end with "want me to push?" — just push.

**Why:** After I committed a komari-web feature and stopped to ask before pushing, the user said "每次顺便帮我做整套的" — they're hands-off and want the whole set done end-to-end every time.

**How to apply:** This is the general rule for all three forks. [[purcarte-theme-build-deploy]] is the more specific (and stricter) PurCarte variant that additionally builds with proxy+replace-registry-host and copies into the backend's preinstalledThemes before committing the backend. Remotes/branches per [[forked-repos-use-xshhhlol]].
