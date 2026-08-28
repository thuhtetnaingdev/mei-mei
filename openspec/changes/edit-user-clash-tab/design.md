## Context

See proposal.md — Why for motivation. Current state that shapes the approach:

- The edit-user modal (`panel_frontend/src/pages/UsersPage.tsx`) has three tabs (Config | Clash | Bandwidth), with a single `<form>` wrapping the Config and Clash panels and Close/Save reachable from both. Form state is lifted to the page component, so unsaved edits already survive tab switches.
- The clash profile is generated server-side by `subscription.GenerateClashProfile(user, nodes, protocolSettings, extraOutbounds)` → YAML, driven by `buildClashProfileConfig`. Group composition logic is intricate: `nodeMode` splits node proxies into primary/fallback by node clash role; `fallback` + `fallbackMode` decide whether AUTO's member list is node proxies, imported proxies, or all; mieru proxies feed `proxyNames` and therefore AUTO (default mode) and the Proxy select group.
- The preview is intentionally a **sample**: the operator wants to see how their settings shape group structure and parameters, not the user's real node inventory. It is generated entirely in the frontend as a pure function of the form state — no network, no backend surface.
- An earlier iteration of this change built a backend preview endpoint (`GenerateClashGroupsPreview`, `POST /users/:id/clash-preview`); that iteration was replaced by the frontend-only sample approach and the backend work was removed.

## Goals / Non-Goals

**Goals:**
- Clash settings get a dedicated tab; Config tab becomes identity/sync only.
- The Clash tab shows a live sample preview of a `proxy-groups` configuration that updates **immediately** as draft settings change — no save, no network.
- The sample mirrors the real generator's group *structure* (which groups exist, which settings map to which fields) while using placeholder member names.

**Non-Goals:**
- No backend API for previews; no server round-trip.
- No real node names or proxy definitions in the preview (sample by design).
- No changes to the create-user modal (stays a single form).
- No changes to the full clash/sing-box download endpoints or their output.

## Decisions

### D1: Preview is a pure frontend function of form state

`buildClashSamplePreview(form)` is a module-level pure function in `UsersPage.tsx` that returns a YAML string. It runs synchronously on every render, so the preview updates instantly when any draft setting changes — no debounce, no request cancellation, no loading/error states.

- **Alternative considered:** server-side preview endpoint reusing `buildClashProfileConfig` — rejected: shows real proxy names (operator wants sample), needs a network round trip and debounce, and adds API surface for a display-only feature.
- **Alternative considered:** static template with interpolated values — same outcome, but a structured generator keeps the group logic (fallback wiring, node mode split, count truncation) in one place.

### D2: The backend preview API is removed

The previously added `GenerateClashGroupsPreview` helper, `clashPreview` handler, route registration, and unit test are deleted. `GenerateClashProfile` remains the only preview-adjacent backend surface.

### D3: Sample content mirrors the real group logic structurally

The generator reproduces `buildClashProfileConfig`'s composition rules at a structural level, with placeholder members:

- Sample pool: a fixed list of role-tagged node samples (`Node-1-Reality`, `Node-1-TUIC`, `Node-2-Hysteria2`, …), imported samples (`Imported-1`, `Imported-2`), and mieru samples (`Mieru-Node-1`, `Mieru-Node-2` when the mieru toggle is on).
- AUTO members follow the real logic: `nodeMode "nodes"` → primary samples; `fallback && fallbackMode "sub_integration"` → node samples; `fallback && fallbackMode "nodes"` → imported samples (mirrors the real generator's behavior); otherwise all samples.
- Fallback-Nodes members mirror the `fbNames` selection (role fallback / imported / node samples) truncated to `fallbackCount`.
- Group parameters map 1:1 from the form: AUTO gets `interval`/`timeout`/`max-failed-times` always, `tolerance` only for `url-test`, `strategy` for `load-balance`; FALLBACK gets `interval`/`timeout`/`max-failed-times`/`lazy`; Fallback-Nodes gets `interval`/`tolerance`.
- Fallback-Nodes and FALLBACK groups are omitted entirely when Clash Fallback is off.

### D4: Render block simplifies

The Clash panel always renders the computed sample in a scrollable monospace `<pre>` with a Copy button. No loading or error branches exist because nothing is asynchronous.

## Risks / Trade-offs

- **Sample structure may drift from the real generator** → The generator mirrors the composition rules and key mapping explicitly, with a comment pointing at `buildClashProfileConfig`; any future generator change should update the sample alongside it. Accepted: the preview is a sample by design.
- **Placeholder names can't show which real nodes land in which group** → Accepted tradeoff of the "sample" requirement; the value is in seeing group structure and parameter wiring reactively.
- **The real generator's `fallbackMode "nodes"` + fallback quirk (AUTO = imported proxies) is reproduced in the sample** → Intentional: the sample shows what the generated config would structurally do.

## Migration Plan

No data migration. The change is additive frontend behavior plus removal of an unreleased backend endpoint (no consumers other than the previous iteration's frontend code, which is removed in the same change).

## Open Questions

None — the sample naming scheme, pool size, and settings coverage were confirmed with the user.
