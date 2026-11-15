---
name: purcarte-button-style-restrained
description: "PurCarte button taste — FINAL approved style is the dark slate gradient (slate-600→800 + inner top highlight, sky-300 icons, delete in red-300 text) with hover that DARKENS; user is on light mode. Plus the CSS-layer gotcha."
metadata:
  node_type: memory
  type: feedback
  originSessionId: 07752cb6-bc7c-4e2a-ae57-84fd17faa430
  modified: 2026-10-14T04:14:37.979Z
---

**Final approved style (2026-10-14):** the user pointed back at a screenshot of the slate version and said "换成这个颜色风格，然后鼠标悬浮时候，颜色加深".
- Look: a dark slate gradient pill (`slate-600 → slate-800`) with a top inner highlight (`inset 0 1px 0 rgb(255 255 255/.18)`), slate-800 border, slate-100 text and sky-300 icons.
- Hover **darkens** rather than lightens. It is done with an `inset 0 0 0 999px rgb(2 6 23/.32)` overlay: an inset shadow transitions smoothly and paints under the text, whereas a gradient can't transition. No lift, no glow.
- Variants in `src/components/ui/button.tsx`:
  - `slate`: used for 终端 and the PingChart 「隐藏全部/显示全部」 and 「四分之一/重置范围」 buttons. PingChart is shared by the home modal, the table row and the detail page.
  - `slateDanger`: used for 删除; same base with red-300 text. Hover darkens too; it does NOT turn red.
- The 负载/延迟 and time-range toggles on the detail page still use the violet `default` variant on purpose.

Path to this choice, all on light mode, which is what the user uses:
1. violet gradient with lift and glow ✗
2. flat black ✗
3. white glass ✗
4. slate gradient, hover lightened (shipped; the user only later called it 丑)
5. gray frosted glass "A", picked from a preview sheet, then replaced
6. back to the slate look with a darkening hover ✓

Taste can swing, so always show previews ([[purcarte-visual-preview-before-ship]]).

**CSS gotcha:** In this build, Tailwind preflight and utilities are *unlayered*, while custom rules in `src/index.css` sit inside `@layer`. So preflight's `button { background-color: transparent; color: inherit }` and `* { border: 0 solid }` beat layered classes. Custom button classes need `!important` on background-color, color, border-color and border width. `background-image`, `box-shadow` and `backdrop-filter` need no `!`. Radix Themes dialogs need `.rt-BaseDialogOverlay { z-index: 60 }` to sit above the z-50 node detail modal (already in index.css). Deploy follows [[purcarte-theme-build-deploy]].
