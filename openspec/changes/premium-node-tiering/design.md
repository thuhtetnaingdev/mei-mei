## Context

See proposal.md — Why for motivation. Current state relevant to the approach:

- All three subscription outputs (base64 links, sing-box JSON, clash YAML) already funnel through a single filter, `filterAvailableNodes(user, nodes)` in `internal/subscription/generator.go`. One change there partitions every output.
- Node sync flows: `SyncAllUsers(users)` (called by `syncActiveUsersBestEffort()` from user create/update/delete handlers) and `syncNodeEnabledState(node)` both build a user list and pass it to `syncNode(node, users)`, which filters per user (`IsTesting && !node.IsTestable`). `expectedUserCount` is derived from the users list actually passed in, so tier filtering needs no extra verification logic.
- `updateNode` (PATCH `/nodes/:id`) does **not** currently trigger any user re-sync — a premium flip on a node needs a new sync trigger there.
- GORM `AutoMigrate` in `internal/db/db.go` adds new columns automatically; no migration script exists for boolean additions.

## Goals / Non-Goals

**Goals:**
- Strict tier partition in both dimensions: subscription outputs and node-sync enforcement.
- Reuse the existing single chokepoint (`filterAvailableNodes`) and the existing sync path (`syncNode`) — no new architectural patterns.
- Same-tier behavior for existing users/nodes is unchanged (flags default to `false` = regular).

**Non-Goals:**
- Per-user custom node selection (previous idea — superseded by tiering).
- Filtering only the sing-box output (tier applies to all outputs).
- Mixed-access models (e.g., premium user also using regular nodes).
- Node-side protocol changes — enforcement is purely which UUIDs each node receives.

## Decisions

1. **Model: boolean flags on existing tables.** Add `Node.Premium bool` (`gorm:"default:false"`) and `User.Premium bool` (`gorm:"default:false"`). Rationale: the partition is a single dimension and booleans mirror the existing `IsTesting`/`IsTestable` pattern. Alternatives: an enum role column (`"regular"`/`"premium"`) — rejected, boolean is simpler and matches the codebase; a join table — rejected, there is no many-to-many relationship.

2. **Subscription partition: extend `filterAvailableNodes`.** Add `if user.IsPremium != node.IsPremium { continue }` to the existing loop. Because `Generate`, `GenerateNodeLinks`, `GenerateSingboxProfile`, and `GenerateClashProfile` all call `filterAvailableNodes`, all three output formats partition with one change. Alternative: a separate tier filter per output — rejected as redundant and error-prone.

3. **Node-sync partition: filter inside `syncNode`.** Add the same tier check next to the existing `user.IsTesting && !node.IsTestable` skip. This covers both `SyncAllUsers` and `syncNodeEnabledState` since both pass through `syncNode`. `expectedUserCount` and sync verification need no changes — they derive from the filtered list. Alternative: filter in `SyncAllUsers` before the per-node loop — rejected because `syncNodeEnabledState` also needs it and per-node context (the node's tier) lives in `syncNode`.

4. **Sync trigger on node premium flip.** In the `updateNode` handler, capture the previous node premium value (read before `NodeService.Update` — the service already reads previous `Enabled`/`IsTestable`), and after a successful update call `syncActiveUsersBestEffort()` only when the flag changed. User create/update/delete already call `syncActiveUsersBestEffort()`, so user flips propagate with no new wiring. Alternative: `syncNodeEnabledState` for just the changed node — rejected: `syncActiveUsersBestEffort` is the established handler-side pattern and keeps all nodes consistent after one admin action.

5. **API input: pointer fields.** `CreateUserInput.Premium *bool`, `UpdateUserInput.Premium *bool`, `UpdateNodeInput.Premium *bool` — pointer semantics match the existing nullable update fields (`Enabled`, `IsTesting`, `IsTestable`) and let create default to `false` when absent.

## Risks / Trade-offs

- [Flag flip is breaking for affected users] → Deliberate and documented in proposal.md as **BREAKING**; the admin action is explicit. Re-sync is best-effort, so there is a short window where stale UUIDs remain valid on a node until re-sync completes.
- [Best-effort re-sync failure leaves stale UUIDs] → Existing sync verification compares `expectedUserCount` vs `appliedUserCount` per node and surfaces mismatches; operator re-syncs. No new failure mode beyond today's.
- [Premium user with zero premium (or zero enabled/testable premium) nodes] → Subscriptions contain no node links; the sing-box profile already degrades gracefully to direct-only (`collectOutboundTags` falls back to `["direct"]`). Documented behavior; admin can see the node list.
- [Node bandwidth attribution shifts when tiers are introduced] → Nodes only account the users they serve, so attribution is automatically correct; no collector changes needed.

## Migration Plan

1. Deploy backend: `AutoMigrate` adds `nodes.premium` and `users.premium` as `NOT NULL DEFAULT false`.
2. Deploy frontend: new checkboxes/badges.
3. No data backfill: existing nodes/users are regular until explicitly flipped.
4. Rollback: remove the tier checks (or ignore the flags) — behavior reverts to today's; columns can stay harmlessly.
