---
title: "bug: DataSource.Degraded() not reset after successful fallback"
labels: ["bug"]
---

## Summary

`DataSource.Degraded()` returns `true` after the primary fails and a fallback succeeds. It should reset to `false` since the system recovered via fallback.

## Steps to Reproduce

1. Create a DataSource with primary + fallback
2. Primary fails → ScanAll falls back to fallback → success
3. Call `Degraded()` → returns `true` (should be `false`)

## Root Cause

In `datasource.go`, after a fallback succeeds, `errCount` is not reset to 0. Only the primary success path resets it.

## Severity

Medium — impacts observability, not correctness. Callers relying on `Degraded()` to decide refresh strategy would get wrong signals.

## Fix

Reset `ds.errCount = 0` on fallback success path.
