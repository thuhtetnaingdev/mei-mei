## 1. Backend — join table

- [x] 1.1 Add `UserSelectedNode` model (`ID`, `UserID`, `NodeID`, unique composite index on user_id+node_id, cascade deletes) in `panel_backend/internal/models/`
- [x] 1.2 Register the model in `AutoMigrate` in `panel_backend/internal/db/db.go` and confirm the table is created on boot

## 2. Backend — user service (selection persistence)

- [x] 2.1 Add `SelectedNodeIDs *[]uint` to `CreateUserInput` and `UpdateUserInput` in `panel_backend/internal/services/user_service.go`
- [x] 2.2 In `UserService.Create`, store the selection for premium users (ignore for non-premium); in `UserService.Update`, replace the selection only when the pointer is non-nil
- [x] 2.3 Ensure selection rows are replaced within the existing user transaction and node-deletion cascades clean them
- [x] 2.4 Include `selectedNodeIds` in the user detail response (`GetByID` / list serialization)

## 3. Backend — sing-box curation

- [x] 3.1 In the sing-box generation path, filter available nodes to the user's selected node IDs when `user.Premium && len(selection) > 0`; empty selection falls through to all nodes
- [x] 3.2 Confirm base64 links (`Generate`/`GenerateNodeLinks`) and clash (`GenerateClashProfile`) are NOT affected by selection

## 4. Backend — one-directional sync filter

- [x] 4.1 In `syncNode` (`panel_backend/internal/services/node_service.go`), change `user.Premium != node.Premium` to `!user.Premium && node.Premium`
- [x] 4.2 Update `TestSyncAllUsersFiltersUsersByNodeTier` to assert premium users are pushed to both premium and regular nodes, and regular users only to regular nodes

## 5. Backend — tests

- [x] 5.1 Sing-box profile: premium user with selection → only selected nodes; empty selection → all nodes
- [x] 5.2 Base64 links and clash profile: premium user with selection → all nodes (not curated)
- [x] 5.3 Regular user: premium nodes excluded from all three outputs (existing partition behavior still covered)
- [x] 5.4 User service: selection persisted on create/update, replaced on update, ignored for regular users

## 6. Frontend

- [x] 6.1 Add `selectedNodeIds: number[]` to the `User` type in `panel_frontend/src/types/index.ts`
- [x] 6.2 `panel_frontend/src/pages/UsersPage.tsx`: node picker (checkbox list of all nodes) shown only for premium users in create and edit dialogs; label it as sing-box profile selection; include `selectedNodeIds` in create/update payloads; load current selection when editing

## 7. Verification

- [x] 7.1 `go test ./...` in `panel_backend/` — all tests pass (excluding the pre-existing `TestImportSubscriptionCreatesUserFromHeader` failure)
- [x] 7.2 `npm run build` in `panel_frontend/` — typecheck and build pass
- [x] 7.3 Manual smoke: premium user with a selection → sing-box profile contains only selected nodes, base64/clash contain all; regular user → no premium nodes anywhere; flipping a node to premium evicts regular users' UUIDs
