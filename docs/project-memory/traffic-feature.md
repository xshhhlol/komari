# Daily traffic feature

Claude session 75b8f7a5-6df4-4a8a-81a7-0edbe45c4fce began 2026-11-13. User approved detail-page daily traffic plus home traffic rankings by date dimension; no billing reset day. Other recommendations accepted.

Implementation: database/traffic accumulates adjacent reports, persists daily usage and cursors transactionally, backfills historical load records, splits gaps across local calendar days. Notifications read the same usage. Application timezone defines day boundaries. Counter resets without reboot rebaseline; reboot detection uses estimated boot time. Historical data has no uptime and is approximate. Queries include pending usage and serialize against flush. Out-of-order samples are ignored.

Public RPC getTrafficDaily/getTrafficRanking and REST /api/records/traffic, /api/traffic/ranking apply existing hidden-node filtering. PurCarte TrafficPanel provides 7/30/90-day stacked bars and day/week/month rankings with previous-period navigation. Auto refresh every minute. Theme source resides in sibling komari-theme-purcarte, artifacts embedded in backend.

Verification: Go tests and production theme build; no live-server deployment or browser visual verification performed during initial implementation. Release and panel upgrade still needed to ship to running server.

2026-11-15 redesign: home ranking lives behind a compact statistics-toolbar button and opens a Radix dialog; never render the ranking panel directly on home. Use a solid light/dark dialog surface, compact rows, IP below node name (admin-only), direction columns on desktop, and five absolute usage color bands (<100 GiB, 100–500 GiB, 500 GiB–1 TiB, 1–5 TiB, ≥5 TiB). Browser checked desktop/mobile with sample data. User reiterated dislike of the original full-width panel.

Latest user correction: visual redesigns must be shown as screenshots and approved before publication. The 2.32.2 absolute-position entry overlapped group pills; next revision uses a normal-flow toolbar with a shrinkable group region and fixed-size action region. Current revision is local preview only, pending user approval.

Preview feedback: user wants a visible text-labelled ranking button, not icon-only. Avoid teal/green palettes such as #0d9488 in ranking; use muted slate/blue-violet and warmer high-usage colors. Provide uncropped screenshots showing the actual entry. Keep this redesign local until approval.

User approved the v4 preview for publication. Color replacement is scoped to traffic-ranking CSS/component; no global palette or unrelated cards/charts changed. The only external layout change is the stats-toolbar flex layout required to prevent button/group overlap.

New pending preview: user requests more color (previous design too gray), still no teal. Ranking response now includes existing semicolon-separated node tags, shown as a separate badge column after node. Brighter blue/violet/gold summary cards and usage colors are scoped to dialog. Desktop/mobile sample-data previews generated; awaiting approval before push/release.

Latest preview refinement: shrink header, toolbar and summary; dialog fixed height with flexible scrolling list so switching day/week/month, loading and empty results never move or resize the window. Show only IPv4, never IPv6, in ranking. Browser geometry checks passed for many/one/zero rows on desktop/mobile. Still pending visual approval.

User rejected the compressed top area: restore the previous spacious title/date/three colored summary cards. Retain fixed window size, independent tags column and IPv4-only rendering. Await approval before publishing.

User approved publication of the restored upper layout with brighter summary cards, separate node tags, IPv4-only display and fixed dialog dimensions. Release this version; do not wait for CI completion.
