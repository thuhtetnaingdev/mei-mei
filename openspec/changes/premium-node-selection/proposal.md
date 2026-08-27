## Why

The premium tiering shipped in `premium-node-tiering` (v0.6.0) is a strict partition: premium users get only premium nodes, regular users get only regular nodes. This is too rigid — the panel wants to sell *curated* premium plans where an admin picks exactly which nodes a premium user receives (premium and regular alike), while keeping premium nodes exclusive to premium users.

## What Changes

- **Keep**: `Node.Premium` flag and its exclusivity role — regular users never see premium nodes in any subscription output, and their UUID is never pushed to premium nodes (enforcement).
- **Keep**: `User.Premium` flag — it now gates a node picker instead of a partition.
- **Modify sync enforcement to one-directional**: today `syncNode` skips users whenever `user.Premium != node.Premium` (both directions). It becomes `!user.Premium && node.Premium` — regular users are still excluded from premium nodes, but premium users' UUIDs remain valid on every node (their selection is cosmetic, not enforced).
- **Add per-user node selection for premium users**: a `user_selected_nodes` join table; admin picks a subset of *all* nodes (premium + regular) in the user create/update forms.
- **Sing-box-only curation**: the selection curates only the sing-box JSON profile. Base64 vless links and the clash YAML profile still list all nodes for premium users.
- **Graceful default**: a premium user with an empty selection (e.g., newly created) receives all nodes in the sing-box profile — same as a regular user's behavior for non-premium nodes — rather than a broken empty profile.

## Capabilities

### New Capabilities
- `node-selection`: Per-user node selection for premium users, applied to the sing-box profile.
- `nodes`: Premium-node flag and tier-based serving — premium nodes exclude regular users in all outputs and at sync time, while serving premium users alongside regular nodes.
- `subscriptions`: Tier + selection behavior across all subscription outputs (regular exclusion everywhere; premium selection in sing-box only).
- `users`: Premium flag meaning — eligible for node selection rather than restricted to premium nodes.

Note: `openspec/specs/` is empty because `premium-node-tiering` was implemented and committed (`a22207a`, tag `v0.6.0`) but never archived. This change therefore introduces these capabilities as new (its deltas define the full target behavior) and supersedes the shipped behavior; archive the two changes together in order (`premium-node-tiering`, then `premium-node-selection`).

## Impact

- **panel_backend**
  - `internal/models/` — new `UserSelectedNode` model; no changes to `User`/`Node` models (flags stay)
  - `internal/db/db.go` — `AutoMigrate` picks up the join table
  - `internal/services/user_service.go` — `selectedNodeIds` in create/update inputs; load/save selections
  - `internal/services/node_service.go` — `syncNode` filter becomes `!user.Premium && node.Premium`
  - `internal/subscription/generator.go` / `singbox_profile.go` — sing-box profile filters to the selected set for premium users; base64/clash untouched
  - `internal/api/router.go` — user responses include `selectedNodeIds`
- **panel_frontend**
  - `src/pages/UsersPage.tsx` — node picker (checkbox list) shown for premium users in create/edit dialogs
  - `src/types/index.ts` — `selectedNodeIds` on User
- **Tests**: sing-box curation tests; one-directional sync filter test; base64/clash-unaffected tests
