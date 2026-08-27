## Purpose

Defines the premium flag on user accounts, which determines the tier of nodes a user is entitled to use.

## ADDED Requirements

### Requirement: User premium flag
The system SHALL allow a user to be marked as premium or regular. Users SHALL default to regular (non-premium) when created or when the flag is not explicitly set. The flag SHALL be editable on both user create and user update.

#### Scenario: Create premium user
- **WHEN** an admin creates a user and sets the premium flag
- **THEN** the user is stored as premium

#### Scenario: Update user tier
- **WHEN** an admin changes a user's premium flag during user update
- **THEN** the stored user tier reflects the change and the panel re-syncs all nodes

#### Scenario: Default flag
- **WHEN** a new user is created without a premium setting
- **THEN** the user is treated as regular (non-premium)
