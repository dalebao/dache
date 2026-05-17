---
title: "bug: RefreshGroup.Start() blocks caller forever"
labels: ["bug"]
state: "fixed"
---

## Summary

`RefreshGroup.Start()` calls `sync.Once.Do()` with an infinite loop, blocking the calling goroutine forever. Start() should return immediately and run the loop in a background goroutine.

## Steps to Reproduce

1. Create a RefreshGroup
2. Call `Start(ctx, fn)` — this never returns
3. Any code after `Start()` is unreachable

## Root Cause

`once.Do(func() { ... infinite for/select loop ... })` executes synchronously. Since the loop never exits, once.Do never returns.

## Fix

Launch `go rg.loop(ctx, fn)` inside `once.Do` so Start returns immediately.

## Status

✅ Fixed in commit `e77f828` (subsequent patch).
