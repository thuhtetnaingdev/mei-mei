## Why

The panel needs a premium tier: premium nodes are a sellable perk. Today every user sees every node in every subscription output, and every user UUID is valid on every node — so a "premium node" is indistinguishable from a regular one. Without per-user tiering, premium access cannot be sold.

## What Changes

- **Node premium flag**: Nodes gain a `Premium` boolean (default `false`), editable in the node create/edit form, shown as a badge in the node list.
- **User premium flag**: Users gain a `Premium` boolean (default `false`), editable in the user create/edit form.
- **Strict tier partition in subscription outputs**: `filterAvailableNodes` filters by tier. Premium users only get premium nodes; regular users only get regular nodes. Applies to all three outputs: base64 vless links, sing-box JSON profile, and clash YAML profile.
- **Node-sync enforcement**: Each node only receives same-tier users during sync — regular users' UUIDs are never pushed to premium nodes, and premium users' UUIDs are never pushed to regular nodes. A regular user's UUID stops authenticating on premium nodes, and vice versa. **BREAKING**: existing UUIDs on nodes change behavior as soon as a flag flips.
- **Immediate propagation**: Changing a user's premium flag re-syncs all nodes (already wired via `syncActiveUsersBestEffort`). Changing a node's premium flag gains a sync trigger (currently `updateNode` does not re-sync).
- **BREAKING**: marking a node premium immediately removes it from every regular user's subscriptions and revokes regular users' auth on it. Marking a user premium immediately restricts their subscription to premium-only nodes.

## Capabilities

### New Capabilities
- `nodes`: Premium flag on nodes and tier-based serving rules (which users each node serves).
- `users`: Premium flag on users.
- `subscriptions`: Tier partition applied to all subscription/profile outputs.

### Modified Capabilities
- None (no existing specs; this is the first spec-driven change).

## Impact

- **panel_backend**
  - `internal/models/node.go` — add `Premium bool`
  - `internal/models/user.go` — add `Premium bool`
  - `internal/db/db.go` — GORM `AutoMigrate` picks up the new columns (no migration script needed; defaults to `false`)
  - `internal/subscription/generator.go` — tier filter in `filterAvailableNodes` (single chokepoint feeding all three outputs)
  - `internal/services/node_service.go` — tier filter in `SyncAllUsers` / `syncNode` / `syncNodeEnabledState` (which users are pushed to each node)
  - `internal/services/user_service.go` — accept `Premium` in `CreateUserInput` / `UpdateUserInput` and persist it
  - `internal/api/router.go` — `updateNode` triggers a user re-sync when `Premium` flips (user create/update already call `syncActiveUsersBestEffort`)
- **panel_frontend**
  - `src/pages/NodesPage.tsx` — "Premium node" checkbox in create/edit forms, badge in list
  - `src/pages/UsersPage.tsx` — "Premium user" checkbox in create/edit dialog
  - `src/types/index.ts` — `premium` on Node and User types
- **Testing**: `subscription/generator_test.go` gains tier-partition cases; node sync tests gain tier-filter cases.
- **Node sync verification**: per-node `expectedUserCount` now counts the tier-filtered user set automatically (it is computed from the users list passed in).
- **Unchanged**: node bandwidth collection/reporting (nodes only account users they serve), miner system, protocol settings sync.
