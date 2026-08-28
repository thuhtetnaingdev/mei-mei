## 1. Backend — remove the preview API

- [x] 1.1 Remove `GenerateClashGroupsPreview` from `panel_backend/internal/subscription/singbox_profile.go`
- [x] 1.2 Remove the `clashPreviewInput` struct, the `clashPreview` handler, and the `POST /users/:id/clash-preview` route registration from `panel_backend/internal/api/router.go`
- [x] 1.3 Remove `TestGenerateClashGroupsPreviewShapesGroupsFromSettings` from `panel_backend/internal/subscription/generator_test.go`
- [x] 1.4 Run `go build ./...` and `go test ./...` in `panel_backend/` and confirm the removal is clean

## 2. Frontend — pure client-side sample preview

- [x] 2.1 Remove `ClashGroupsPreviewRequest` and `getClashGroupsPreview` from `panel_frontend/src/api/client.ts` and their import in `UsersPage.tsx`; re-type `buildClashSettingPayload` without the removed type
- [x] 2.2 Add a module-level pure function `buildClashSamplePreview(form)` in `UsersPage.tsx` that renders a sample `proxy-groups` YAML string reactive to every clash setting: sample member pool with roles (e.g. Node-1-Reality, Node-1-TUIC, Node-2-Hysteria2…), imported samples, mieru samples when enabled; AUTO members per nodeMode/fallback/fallbackMode; Fallback-Nodes members per fallbackMode truncated to fallbackCount; FALLBACK group when fallback is on; group parameters mapped from the form (interval/tolerance/timeout/max-failed-times/lazy/strategy); no real node data
- [x] 2.3 Remove the preview state and the debounced fetch effect (`clashPreview`, `clashPreviewLoading`, `clashPreviewError`, `clashPreviewSeqRef`) and compute the preview synchronously on render
- [x] 2.4 Update the Clash panel render block: always show the computed sample in a scrollable monospace `<pre>` with a Copy button; no loading or error branches
- [x] 2.5 Run `npm run build` in `panel_frontend/` (tsc + vite) and fix any type errors

## 3. Validation

- [x] 3.1 Grep the repo for lingering references to the removed API (`getClashGroupsPreview`, `clash-preview`, `clashPreview`, `GenerateClashGroupsPreview`) and confirm none remain in source
- [x] 3.2 Code-level review: the preview reflects draft changes immediately with no network calls, uses only sample names, and respects fallback toggle/count and mieru toggle
