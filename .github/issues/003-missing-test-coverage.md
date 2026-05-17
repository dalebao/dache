---
title: "missing: test coverage for View, DataSource, connector sub-packages"
labels: ["enhancement", "testing"]
---

## Summary

The following packages/types have zero test coverage:

- `View` — Add, Refresh, Get, QueryView, version stamping
- `DataSource` — ScanAll with fallback, Degraded()
- `conn/sql` — SQL connector implementation
- `conn/redis` — Redis connector implementation
- `conn/memcache` — Memcached connector implementation

## Impact

Without tests, regressions in the refresh and fallback paths cannot be detected. These are the core consistency mechanisms.

## Suggested Coverage

| Component | Tests Needed |
|-----------|-------------|
| View.Add + Refresh | Load all tables atomically |
| View version stamp | Version increments after refresh |
| DataSource fallback | Primary fails → fallback succeeds → data loaded |
| SQL connector | ScanAll with struct field mapping |
