## Purpose

Controls which nodes appear in each subscription output: regular users are excluded from premium nodes in all outputs, and premium users' sing-box profile is curated to their selected nodes.

## ADDED Requirements

### Requirement: Regular users receive only non-premium nodes
The system SHALL exclude premium nodes from all subscription outputs of regular users — base64 vless links, sing-box JSON profile, and clash YAML profile.

#### Scenario: Regular user base64 links exclude premium nodes
- **WHEN** a regular user requests the base64 subscription
- **THEN** only non-premium node links are included

#### Scenario: Regular user sing-box profile excludes premium nodes
- **WHEN** a regular user requests the sing-box profile
- **THEN** only non-premium node outbounds are included

#### Scenario: Regular user clash profile excludes premium nodes
- **WHEN** a regular user requests the clash profile
- **THEN** only non-premium node proxies are included

### Requirement: Premium user sing-box profile is curated to selection
The system SHALL filter the sing-box JSON profile of a premium user to their selected nodes. The base64 links and clash profile of a premium user SHALL remain unfiltered (all nodes).

#### Scenario: Premium user sing-box profile contains only selected nodes
- **WHEN** a premium user has selected nodes and requests the sing-box profile
- **THEN** only the selected nodes appear as outbounds

#### Scenario: Premium user base64 and clash profiles are not curated
- **WHEN** a premium user has selected nodes and requests the base64 subscription or clash profile
- **THEN** all nodes appear regardless of selection

#### Scenario: Empty selection behaves like all nodes
- **WHEN** a premium user has no selected nodes and requests the sing-box profile
- **THEN** all available nodes appear
