## Purpose

Lets admins assign a curated subset of nodes to premium users, stored per user and applied to the sing-box profile.

## ADDED Requirements

### Requirement: Admin selects nodes for premium users
The system SHALL allow an admin to select any subset of nodes (premium and regular) for a premium user, on both user create and user update. The selection SHALL be stored per user and SHALL be returned in the user's detail response.

#### Scenario: Set selection on create
- **WHEN** an admin creates a premium user with a set of selected node ids
- **THEN** the selection is stored with the user

#### Scenario: Update selection
- **WHEN** an admin changes a premium user's selected node ids during user update
- **THEN** the stored selection is replaced with the new set

#### Scenario: Selection visible in user response
- **WHEN** the panel returns a user's details
- **THEN** the user's selected node ids are included

#### Scenario: Selection ignored for regular users
- **WHEN** an admin submits selected node ids for a regular (non-premium) user
- **THEN** the selection is not stored and has no effect

#### Scenario: Node deletion cleans selection
- **WHEN** a node that was selected for a user is deleted
- **THEN** the deleted node's id is removed from the user's selection without breaking the user record
