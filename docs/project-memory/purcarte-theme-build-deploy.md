---
name: purcarte-theme-build-deploy
description: "Standing procedure — any task that changes the PurCarte theme MUST auto-run the full build→embed-into-backend→push pipeline, not just edit source"
metadata: 
  node_type: memory
  type: project
  originSessionId: 78da490c-3944-4862-befe-c849c8087cf7
---

The user runs the **PurCarte** theme as their Komari public dashboard and is **completely hands-off** with its project. See also [[forked-repos-use-xshhhlol]].

**STANDING PROCEDURE — do this automatically, do NOT stop after editing source, do NOT ask whether to build/deploy.** Whenever a task involves changing the PurCarte theme, run all steps end to end:

1. **Edit source** in `/Users/asche/GolandProjects/komari-theme-purcarte` (React 19 + Vite + Tailwind; a二开 of `Montia37/Komari-theme-purcarte`, `origin` is upstream/read-only).
2. **Build** (this machine needs the proxy + official registry override + relaxed peers, or npm hangs on the dead npmmirror):
   ```
   cd /Users/asche/GolandProjects/komari-theme-purcarte
   export all_proxy=http://127.0.0.1:1080 http_proxy=http://127.0.0.1:1080 https_proxy=http://127.0.0.1:1080
   npm install --registry=https://registry.npmjs.org --replace-registry-host=always --legacy-peer-deps --no-audit --no-fund
   npm run build      # -> dist/  (long-running: run in background, watch node_modules grow)
   ```
   `--replace-registry-host=always` is essential: yarn.lock pins packages to registry.npmmirror.com (dead from here) and npm's default `replace-registry-host=npmjs` won't rewrite them.
3. **Embed into the backend** (this is what "deploys" it): copy `komari-theme.json` + `preview.png` + `dist/` into the komari backend at `web/public/preinstalledThemes/PurCarte/` (overwrite). The backend already `//go:embed preinstalledThemes` and on startup `SyncPreinstalledThemes()` copies it into `./data/theme/PurCarte/` (files only).
4. **Commit + push**: commit the theme repo locally (preserve二开 history) AND commit+push the komari **backend** (`xshhhlol/komari` main) with the updated artifacts.
5. **Tell the user the only manual steps**: cut a komari Release, then run the upgrade script on the server. First time only: set current theme = PurCarte in the panel.

**Guardrails (never violate):**
- Personalization lives in the Komari DB (`ThemeConfiguration`, key `short=PurCarte`), separate from theme files; the boot-sync overwrites files only and must never touch DB settings or the active-theme selection. Redeploy therefore never resets the user's settings.
- NEVER put PurCarte into `komari-web` or `web/public/defaultTheme/`: `/admin` and `/terminal` are hardcoded to the embedded defaultTheme and PurCarte has no admin/terminal pages → would break them.
- Don't change the theme `short` (`PurCarte`).
- `web/public/.gitignore` is scoped to `/defaultTheme/` so `preinstalledThemes/` (incl. its `dist/`) is committed normally.

**Customization already shipped**: node IP display — toggle `showNodeIP` (in `komari-theme.json` + `src/config/default.ts`, default off), gated on login in `src/hooks/useNodeCommons.ts`, rendered in NodeGrid/NodeCompact/NodeTable. `NodeData` has `ipv4?`/`ipv6?` (backend `/api/nodes` returns these only for logged-in users). Full doc lives in the theme repo at `二开与部署说明.md`.
