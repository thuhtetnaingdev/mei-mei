## 1. Backend — model fields

- [x] 1.1 Add `Premium bool` field (gorm `default:false`) to `models.Node` in `panel_backend/internal/models/node.go`
- [x] 1.2 Add `Premium bool` field (gorm `default:false`) to `models.User` in `panel_backend/internal/models/user.go`
- [x] 1.3 Confirm `AutoMigrate` in `panel_backend/internal/db/db.go` picks up both new columns (start panel backend, check SQLite schema has `nodes.premium` and `users.premium` default false)

## 2. Backend — subscription tier partition

- [x] 2.1 Add tier check to `filterAvailableNodes` in `panel_backend/internal/subscription/generator.go`: skip a node when `user.IsPremium != node.IsPremium`
- [x] 2.2 Add tests in `panel_backend/internal/subscription/generator_test.go`:
  - premium user → only premium nodes in base64 links, sing-box profile, clash profile
  - regular user → only regular nodes in all three outputs
  - tier filter composes with existing filters (disabled node, bandwidth-exceeded node, testable-only for testing users)

## 3. Backend — node-sync enforcement

- [x] 3.1 Add tier check in `syncNode` in `panel_backend/internal/services/node_service.go` next to the existing `user.IsTesting && !node.IsTestable` skip: skip a user when `user.IsPremium != node.Premium`
- [x] 3.2 Add/update sync tests covering: premium node receives only premium users, regular node receives only regular users, and `expectedUserCount` reflects the tier-filtered list
- [x] 3.3 In the `updateNode` handler (`panel_backend/internal/api/router.go`), capture the node's previous `Premium` value before update and call `syncActiveUsersBestEffort()` after a successful update when the flag flipped
- [x] 3.4 Confirm user create/update/delete handlers already trigger `syncActiveUsersBestEffort()` so user premium flips propagate (no code change expected; verify by reading the handlers)

## 4. Backend — API inputs

- [x] 4.1 Add `Premium *bool` to `CreateUserInput` and persist it in `UserService.Create` (default false when absent)
- [x] 4.2 Add `Premium *bool` to `UpdateUserInput` and persist it in `UserService.Update`
- [x] 4.3 Add `Premium *bool` to `UpdateNodeInput` and persist it in `NodeService.Update`

## 5. Frontend

- [ ] 5.1 Add `premium: boolean` to the `Node` and `User` types in `panel_frontend/src/types/index.ts`
- [ ] 5.2 `panel_frontend/src/pages/NodesPage.tsx`: "Premium node" checkbox in the node create form and edit form (next to "Testable node"), premium badge in the node list rows, include `premium` in create/update payloads
- [ ] 5.3 `panel_frontend/src/pages/UsersPage.tsx`: "Premium user" checkbox in the create/edit dialog (next to "Testing user"), include `premium` in create and update payloads

## 6. Verification

- [x] 6.1 Run `go test ./...` in `panel_backend/` — all tests pass
- [x] 6.2 Run `npm run build` in `panel_frontend/` — typecheck and build pass
- [x] 6.3 Manual smoke test: mark one node premium, create one premium and one regular user; verify (a) subscription outputs partition by tier, (b) node sync pushes only matching-tier users to each node, (c) flipping a node's premium flag re-syncs and removes the other tier's UUIDs
