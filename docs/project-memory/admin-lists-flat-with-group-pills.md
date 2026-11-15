---
name: admin-lists-flat-with-group-pills
description: "komari-web admin tables — keep the list flat; groups are handled by the group-pill filter above, never by grouped sections/headers in the table"
metadata:
  node_type: memory
  type: feedback
  originSessionId: f72f9ce3-9a5f-46d2-8076-cc2e2975de49
  modified: 2026-11-07T02:21:39.257Z
---

In komari-web admin pages (e.g. 通知 → 节点通知), node lists stay a **flat table sorted by weight**. Grouping is done only via the group-pill filter row above the table (shared `GroupFilterPills` + `utils/nodeGroups`). Don't add per-group section rows, collapsible groups, or a "按分组显示" toggle.

**Why:** 2026-11-07 I shipped 2.31.0 with grouped section rows in the 节点通知 table; the user said "下面列表不要再分组了，上方已经有分组了" — the pills already cover grouping, so sections were redundant noise (the real panel has ~220 nodes in ~15 groups).

**How to apply:** when a request says "支持分组展示/按分组" for an admin list, implement it as the pill filter (with counts), not as table sections. Delivery per [[forked-repos-full-pipeline-no-ask]].
