## Purpose

Defines node tiering: each node is either premium or regular, and a node only serves users of its own tier, enforced at node-sync time.

## ADDED Requirements

### Requirement: Node premium flag
The system SHALL allow a node to be marked as premium or regular. Nodes SHALL default to regular (non-premium) when created or when the flag is not explicitly set. The flag SHALL be editable through the node create and node update forms, and SHALL be visible in the node list.

#### Scenario: Mark node as premium
- **WHEN** an admin marks a node as premium
- **THEN** the node is stored as premium and shown with a premium badge in the node list

#### Scenario: Default flag
- **WHEN** a new node is created without a premium setting
- **THEN** the node is treated as regular (non-premium)

### Requirement: Tier-based node serving
A node SHALL only serve users of the same tier: premium nodes SHALL only receive premium users, and regular nodes SHALL only receive regular users. A user's UUID SHALL NOT be valid on a node of a different tier.

#### Scenario: Premium node receives only premium users
- **WHEN** the panel pushes active users to a premium node
- **THEN** only premium users' UUIDs are included in the node's configuration

#### Scenario: Regular node receives only regular users
- **WHEN** the panel pushes active users to a regular node
- **THEN** only regular users' UUIDs are included in the node's configuration

#### Scenario: Regular user cannot authenticate on premium node
- **WHEN** a regular user's UUID was previously configured on a node that is now marked premium, and the panel re-syncs
- **THEN** the regular user's UUID is removed from the premium node and no longer authenticates

#### Scenario: Tier change triggers re-sync
- **WHEN** a node's premium flag is changed
- **THEN** the panel re-syncs users to nodes so the serving rules reflect the new tier

#### Scenario: Tier filter composes with testability
- **WHEN** a user is both a testing user and premium, and the node is not testable
- **THEN** the user's UUID is not included regardless of tier match
