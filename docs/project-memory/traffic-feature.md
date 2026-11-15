# Daily traffic feature

Claude session 75b8f7a5-6df4-4a8a-81a7-0edbe45c4fce began 2026-11-13. User approved detail-page daily traffic plus home traffic rankings by date dimension; no billing reset day. Other recommendations accepted.

Implementation: database/traffic accumulates adjacent reports, persists daily usage and cursors transactionally, backfills historical load records, splits gaps across local calendar days. Notifications read the same usage. Application timezone defines day boundaries. Counter resets without reboot rebaseline; reboot detection uses estimated boot time. Historical data has no uptime and is approximate. Queries include pending usage and serialize against flush. Out-of-order samples are ignored.

Public RPC getTrafficDaily/getTrafficRanking and REST /api/records/traffic, /api/traffic/ranking apply existing hidden-node filtering. PurCarte TrafficPanel provides 7/30/90-day stacked bars and day/week/month rankings with previous-period navigation. Auto refresh every minute. Theme source resides in sibling komari-theme-purcarte, artifacts embedded in backend.

Verification: Go tests and production theme build; no live-server deployment or browser visual verification performed during initial implementation. Release and panel upgrade still needed to ship to running server.
