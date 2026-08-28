## MODIFIED Requirements

### Requirement: Edit-user modal separates configuration from bandwidth

When the operator opens the edit-user modal, the panel SHALL present the modal as three labeled tabs — "Config", "Clash", and "Bandwidth" — with "Config" selected by default. The Config tab SHALL show the user's identity and sync controls (email, notes, enabled, testing, premium and node selection, sub integration) and SHALL NOT show clash settings or bandwidth controls. The Clash tab SHALL show all clash profile settings (node mode, AUTO group settings, fallback settings, mieru toggle) and the clash sample preview, and SHALL NOT show identity, sync, or bandwidth controls. The Bandwidth tab SHALL show all bandwidth-related controls: the main wallet balance indicator, the add-bandwidth form (or the testing-mode notice for testing users), and the bandwidth history with per-entry actions.

#### Scenario: Edit modal opens on the Config tab

- **WHEN** the operator clicks Edit on a user row
- **THEN** the edit modal opens with the Config tab selected and the user's identity and sync controls visible

#### Scenario: Switching to the Clash tab

- **WHEN** the operator selects the Clash tab in the edit modal
- **THEN** the modal shows the clash profile settings and the clash sample preview, and hides identity, sync, and bandwidth controls

#### Scenario: Switching to the Bandwidth tab

- **WHEN** the operator selects the Bandwidth tab in the edit modal
- **THEN** the modal shows the wallet balance, the add-bandwidth form (or testing-mode notice), and the bandwidth history with per-entry actions, and hides configuration and clash controls

#### Scenario: Configuration controls are not shown on the Bandwidth tab

- **WHEN** the operator is viewing the Bandwidth tab in the edit modal
- **THEN** no configuration controls (email, notes, enabled, testing, premium, node selection, sub integration) and no clash settings or clash preview are displayed

#### Scenario: Bandwidth controls are not shown on the Config tab

- **WHEN** the operator is viewing the Config tab in the edit modal
- **THEN** no bandwidth controls (wallet balance, add-bandwidth form, bandwidth history) are displayed

#### Scenario: Bandwidth controls are not shown on the Clash tab

- **WHEN** the operator is viewing the Clash tab in the edit modal
- **THEN** no bandwidth controls (wallet balance, add-bandwidth form, bandwidth history) are displayed

#### Scenario: Clash settings are not shown on the Config tab

- **WHEN** the operator is viewing the Config tab in the edit modal
- **THEN** no clash settings (node mode, AUTO group settings, fallback settings, mieru toggle) and no clash preview are displayed

#### Scenario: Unsaved configuration survives a tab switch

- **WHEN** the operator edits a clash setting on the Clash tab without saving, switches to the Config tab, and switches back to the Clash tab
- **THEN** the edited clash setting values are still present

#### Scenario: Testing user sees testing notice on Bandwidth tab

- **WHEN** the operator opens the edit modal for a testing user and selects the Bandwidth tab
- **THEN** the modal shows the testing-mode notice instead of the add-bandwidth form, and still shows the bandwidth history

## ADDED Requirements

### Requirement: Clash tab previews a sample proxy-groups configuration

The panel SHALL display, on the Clash tab of the edit-user modal, a live sample preview of a proxy-groups configuration that reflects the current draft clash settings. The preview SHALL update immediately, without saving and without any network request, whenever a draft value that affects group composition or group behavior changes. The preview SHALL use sample proxy names (for example, "Node-1-Reality" or "Imported-1") and SHALL NOT contain real node names, proxy definitions, rules, credentials, or connection details.

#### Scenario: Editing a setting updates the preview immediately

- **WHEN** the operator changes a clash setting (for example, Auto Interval or Fallback Max Failed) on the Clash tab without saving
- **THEN** the preview updates immediately to show the new value in the corresponding group definition, without a save or network request

#### Scenario: Preview uses sample proxy names

- **WHEN** the preview is displayed for any user
- **THEN** every group member list contains only sample placeholder names (for example, "Node-1-Reality" or "Imported-1") and no real node names

#### Scenario: Fallback toggle controls the fallback groups

- **WHEN** the operator enables Clash Fallback on the Clash tab
- **THEN** the preview shows the Fallback-Nodes and FALLBACK groups; when the toggle is disabled, the preview omits them

#### Scenario: Fallback member list respects the count setting

- **WHEN** the operator sets the fallback count to a value smaller than the sample pool
- **THEN** the Fallback-Nodes member list in the preview contains at most that many entries

#### Scenario: Mieru toggle adds sample mieru members

- **WHEN** the operator enables the Mieru Protocol toggle on the Clash tab
- **THEN** the preview's group member lists include sample mieru entries (for example, "Mieru-Node-1")

#### Scenario: Preview contains groups only

- **WHEN** the preview is displayed for any user
- **THEN** the preview contains only proxy-group definitions and does not contain proxy definitions, rules, or any proxy credentials or server addresses
