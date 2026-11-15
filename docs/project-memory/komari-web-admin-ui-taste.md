---
name: komari-web-admin-ui-taste
description: "komari-web admin taste — restrained nn-* style (2.31.1 节点通知, extended to 服务器页 + sidebar + topbar in 2.31.2): settings in dialogs, selection-only floating bar, neutral buttons, accent fixed indigo, light/dark direct toggle (no system), no search box on 服务器页 (browser find)"
metadata:
  node_type: memory
  type: feedback
  originSessionId: f72f9ce3-9a5f-46d2-8076-cc2e2975de49
  modified: 2026-11-07T03:15:11.056Z
---

The approved look for komari-web admin pages was introduced in 2.31.1 (通知 → 节点通知) and extended to the whole admin frame and the 服务器 page in 2.31.2.
- Shared styles: `src/components/admin/admin-ui.css` (`nn-*` classes, loaded by AdminPanelBar).
- Frame and Radix button skin: `src/global.css`.

The user rejected the first version as "太乱了、太啰嗦、按钮和组件好丑没质感". What they then approved:
- **Header**: a title plus one muted line of stats. Secondary settings sit behind one button and open in a **dialog**, never as an always-visible card.
- **Toolbar** (节点通知): search, then one 「筛选」 dropdown with a count badge. The match count appears only while filtering.
- **No explanatory paragraphs** on the page. Short descriptions go inside dialogs, as settings rows.
- **Batch actions**: a floating bar that appears only after rows are selected. It is light frosted glass in light mode and a dark slate gradient in dark mode ("都要，刚好白天和黑夜模式").
- **Controls**:
  - thin-border neutral buttons; primary buttons use a vertical accent gradient with an inner highlight;
  - a table card with rounded corners and a soft shadow, and light headers;
  - monospace IPs, with the copy icon shown only on row hover;
  - gray action icons, with delete kept red.
- **Frame**:
  - plain light gray background and a white content card;
  - sidebar items in gray text, the active one with a soft accent tint (no gradient glow), sub-menus with a guide line;
  - the version as a small badge, top-bar icons as thin-border squares.
- **服务器 page has no search box.** The user searches with the browser's Cmd/Ctrl+F ("我是通过浏览器的搜索就行了"). So keep full text such as the complete IPv6 in the DOM, and truncate it only visually with CSS.
- **Theme**: a single button toggles light ↔ dark directly; there is no "system"/auto mode ("点击直接切换，不要自动模式").
- **Accent color**: fixed to indigo (`ACCENT_COLOR`). The ColorSwitch picker was removed ("色系功能用不上"); don't reintroduce it.

**How to apply:** new or reworked admin pages follow this pattern and the flat-list rule in [[admin-lists-flat-with-group-pills]]. For look-and-feel changes, render previews first, as in [[purcarte-visual-preview-before-ship]], then ship after the user picks. Pipeline: [[forked-repos-full-pipeline-no-ask]].
