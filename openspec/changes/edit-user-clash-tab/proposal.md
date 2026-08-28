## Why

The edit-user modal's Config tab mixes identity/sync fields with the 15-field clash settings block, and operators cannot see what their clash settings produce without saving and downloading the full profile. Splitting clash settings into their own tab makes configuration findable, and a live groups-only preview shows exactly how the current draft settings shape the generated clash proxy-groups — without exposing proxy credentials.

## What Changes

- The edit-user modal gains a third tab, **Clash**, positioned between **Config** and **Bandwidth**. Only the edit modal changes; the create-user modal stays a single form.
- The entire clash settings block (node mode, AUTO group settings, fallback settings, mieru toggle) moves from the Config tab to the Clash tab. The Config tab keeps only identity and sync controls (email, notes, enabled, testing, premium + node selection, sub integration, testing notice).
- The Clash tab shows a **live sample preview** of a proxy-groups configuration that reflects the current draft clash settings. The preview is generated entirely in the frontend as a pure function of the form state, updates immediately (no network request, no saving), and uses sample proxy names — it never shows real node names or credentials.
- Form structure: a single form spans the Config and Clash panels, with Close/Save actions reachable from both tabs; unsaved edits are preserved across tab switches (state is already lifted).
- Cross-tab help text is updated so gating messages reference the tab where the enabling control lives (e.g. "Requires 'Sub Integration' to be enabled in the Config tab").

## Capabilities

### New Capabilities
<!-- None — this change extends the existing user-management capability. -->

### Modified Capabilities
- `user-management`: The edit-user modal requirement changes from two tabs to three — clash settings move out of the Config tab into a new Clash tab, and the Clash tab gains a live, frontend-generated sample preview of a proxy-groups configuration based on draft settings. No backend API is involved.

## Impact

- `panel_frontend/src/pages/UsersPage.tsx` — tab state type widened to `"config" | "clash" | "bandwidth"`, a third tab button, the clash settings block relocated into a new tab panel, a pure frontend sample-preview generator, and the preview render block.
- `panel_frontend/src/api/client.ts` — no changes needed for this change.
- No database schema changes; no changes to the subscription/download endpoints, the node sync behavior, or the panel backend.
