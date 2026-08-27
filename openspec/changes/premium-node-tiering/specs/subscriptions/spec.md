## Purpose

Controls which nodes appear in a user's subscription outputs based on user and node tier, ensuring premium nodes are only visible to premium users.

## ADDED Requirements

### Requirement: Tier partition in subscription outputs
The system SHALL filter nodes by tier in all subscription outputs — base64 vless links, the sing-box JSON profile, and the clash YAML profile. A premium user SHALL receive only premium nodes; a regular user SHALL receive only regular nodes. The tier filter SHALL apply in addition to the existing filters (user enabled, testing/testable, node enabled, node bandwidth).

#### Scenario: Premium user receives only premium nodes
- **WHEN** a premium user requests any subscription output
- **THEN** only premium nodes appear, and no regular node links or outbounds are included

#### Scenario: Regular user receives only regular nodes
- **WHEN** a regular user requests any subscription output
- **THEN** only regular nodes appear, and no premium node links or outbounds are included

#### Scenario: Tier applies to all output formats
- **WHEN** a user requests the base64 subscription, the sing-box profile, or the clash profile
- **THEN** the tier filter is applied consistently across all three

#### Scenario: Existing filters still apply
- **WHEN** a node matching the user's tier is disabled or has exceeded its bandwidth limit
- **THEN** that node is still excluded from the user's subscription outputs
