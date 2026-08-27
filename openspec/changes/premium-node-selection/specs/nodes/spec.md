## Purpose

Defines premium-node exclusivity: nodes marked premium are never visible to regular users and never serve them, while premium users are served by all nodes and may select a curated subset.

## ADDED Requirements

### Requirement: Premium nodes exclude regular users from all outputs
A node marked premium SHALL be excluded from every subscription output of a regular user — base64 vless links, the sing-box JSON profile, and the clash YAML profile. Regular users SHALL NOT receive premium node links or outbounds in any output.

#### Scenario: Regular user sees only regular nodes everywhere
- **WHEN** a regular user requests the base64 subscription, the sing-box profile, or the clash profile
- **THEN** no premium node appears in any of the three outputs

#### Scenario: Premium nodes remain in premium user outputs
- **WHEN** a premium user requests the base64 subscription or the clash profile
- **THEN** both premium and regular nodes appear

### Requirement: Premium nodes do not serve regular users
The panel SHALL NOT push a regular user's UUID to a premium node during node sync. A regular user's UUID SHALL NOT authenticate on a premium node.

#### Scenario: Regular user UUID removed from premium node on re-sync
- **WHEN** a node is marked premium and the panel re-syncs
- **THEN** regular users' UUIDs are removed from that node and no longer authenticate

#### Scenario: Premium user UUID remains on all nodes
- **WHEN** the panel pushes active users to a premium or regular node
- **THEN** a premium user's UUID is included on every node regardless of node tier
