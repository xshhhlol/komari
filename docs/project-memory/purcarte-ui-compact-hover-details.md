---
name: purcarte-ui-compact-hover-details
description: "PurCarte UI preference — keep cards compact (one stat line), put secondary stats in hover; new view-altering toggles default OFF"
metadata: 
  node_type: memory
  type: feedback
  originSessionId: 605d2931-c6ec-40ce-a113-fc8b0b8baefd
  modified: 2026-09-15T08:52:23.597Z
---

Keep PurCarte ping task cards compact: name + a single stat line (now `P95 | 丢包率`). Secondary numbers (P50/P95/P99, latest value) belong in a hover card, not in extra card lines. New toggles that change how a chart looks (e.g. 截断尖峰 Y-axis clipping) ship **default off**.

**Why:** 2026-09-15 I added a third P50/P95/P99 line to every card and made 截断尖峰 default on; the user said the card area got too big, asked for P95 in the old "latest value" slot, percentiles on hover, and clipping off by default.

**How to apply:** for future PurCarte UI additions, prefer hover/tooltip for secondary stats and default-off for new view-changing switches. Delivery still follows [[purcarte-theme-build-deploy]].
