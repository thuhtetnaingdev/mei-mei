## Context

See proposal.md — Why. Current state: `premium-node-tiering` shipped a strict two-directional partition (commit `a22207a`, tag `v0.6.0`): `filterAvailableNodes` excludes mismatched tiers in all subscription outputs, and `syncNode` skips users when `user.Premium != node.Premium` (both directions). `Node.Premium`, `User.Premium`, the node UI, and the `updateNode` premium-flip sync trigger are all in place and stay.

The new direction adds per-user selection for premium users (curating only the sing-box profile) while keeping premium-node exclusivity for regular users.

## Goals / Non-Goals

**Goals:**
- Premium nodes stay exclusive to premium users: excluded from all regular-user outputs and never serving regular users' UUIDs.
- Premium users get a manual node picker (subset of all nodes) that curates only the sing-box JSON profile.
- Build on the shipped tiering — keep flags, UI, and sync plumbing; change the filter semantics and add selection.

**Non-Goals:**
- Enforcement of the premium selection (a premium user's UUID stays valid on unselected nodes — selection is cosmetic).
- Curating base64 links or the clash profile for premium users.
- Selection for regular users.
- Removing `Node.Premium` or the regular-side exclusion.

## Decisions

1. **Join table `user_selected_nodes`.** Explicit model `UserSelectedNode { ID, UserID, NodeID }` with a unique composite index on `(user_id, node_id)` and `OnDelete:CASCADE` on both FKs. Rationale: matches the codebase's explicit join models (`UserBandwidthNodeUsage`) and makes node-deletion cleanup automatic. Alternative: GORM `many2many` on `User.SelectedNodes` — rejected, the codebase avoids implicit join models and the explicit table is queryable/testable. Alternative: JSON `[]uint` column — rejected, not relational and breaks cascade cleanup.

2. **One-directional sync filter.** In `syncNode`, change `if user.Premium != node.Premium { continue }` to `if !user.Premium && node.Premium { continue }`. Rationale: regular users must never be served by premium nodes, but premium users are valid on every node (their selection is not enforced). The existing `updateNode` premium-flip sync trigger stays — flipping a node to premium must still evict regular users' UUIDs from it.

3. **Sing-box-only curation.** In the sing-box generation path only, filter `filterAvailableNodes` output to the user's selected node IDs when `user.Premium && len(selectedIDs) > 0`. Base64 links (`Generate`/`GenerateNodeLinks`) and clash (`GenerateClashProfile`) are untouched. Rationale: the picker is a presentation concern for the sing-box client, the primary client artifact.

4. **Empty selection = all nodes.** A premium user with no selection behaves like a regular user (all available non-premium... no — all available nodes, since premium users see all nodes). This avoids an empty/broken sing-box profile on user creation. Rationale: graceful default; admins curate deliberately.

5. **API shape.** `CreateUserInput.SelectedNodeIDs []uint` and `UpdateUserInput.SelectedNodeIDs []uint`; `UpdateUserInput` uses a `*[]uint`-style sentinel or a separate "replace" flag so an omitted field doesn't wipe the selection (decision: use `*[]uint` pointer — nil = don't touch, non-nil = replace). Selections are ignored (not stored) for non-premium users. The user detail response includes `selectedNodeIds`. Rationale: pointer semantics match the existing nullable update inputs.

6. **Selection storage and validation.** On create/update of a premium user, validate node IDs exist (GORM FK constraint on insert will surface invalid IDs as errors) and replace the whole set within the existing user transaction. Deleting a node cascades its selection rows (spec requirement).

## Risks / Trade-offs

- [Asymmetry confusion: premium user's base64/clash list all nodes while sing-box is curated] → Intentional per spec; documented in the UI (picker labeled as sing-box profile selection) and in the access dialog where the sing-box URL is primary.
- [Selection replaces whole set — a client omitting selectedNodeIds on update would wipe it] → Mitigated by `*[]uint` semantics: absent field leaves selection untouched; the frontend always sends the current set.
- [Filtering only sing-box means `filterAvailableNodes` stays global while selection is local] → Kept as a dedicated filter inside the sing-box path rather than changing the shared filter; no risk to other outputs.
- [v0.6.0 already ships the two-directional partition] → The next release supersedes it; behavior converges to the one-directional model once this change deploys.

## Migration Plan

1. Deploy backend: `AutoMigrate` creates `user_selected_nodes`; existing columns untouched.
2. Deploy frontend: picker UI in UsersPage.
3. No data backfill — existing premium users have empty selections (all nodes in sing-box until curated).
4. Rollback: revert the one-directional filter and ignore the join table; flags and UI remain harmless.
