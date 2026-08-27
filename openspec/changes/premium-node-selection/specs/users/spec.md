## Purpose

Defines the premium flag on user accounts: premium users are eligible for per-user node selection and are served by all nodes, while regular users are limited to non-premium nodes.

## ADDED Requirements

### Requirement: User premium flag
The system SHALL allow a user to be marked as premium or regular. Users SHALL default to regular (non-premium). The flag SHALL be editable on user create and user update.

#### Scenario: Create premium user
- **WHEN** an admin creates a user with the premium flag set
- **THEN** the user is stored as premium and eligible for node selection

#### Scenario: Default flag
- **WHEN** a new user is created without a premium setting
- **THEN** the user is treated as regular (non-premium) with no node selection

### Requirement: Premium users receive all nodes by default
A premium user SHALL receive premium and regular nodes alike in their base64 links and clash profile, and in their sing-box profile when no selection exists. A regular user SHALL receive only non-premium nodes.

#### Scenario: Premium user without selection gets all nodes in sing-box
- **WHEN** a premium user has no selected nodes and requests the sing-box profile
- **THEN** all available nodes (premium and regular) appear
