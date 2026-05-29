// Package roles defines the canonical role strings used across the platform.
// All call sites that accept or compare role values must import these constants
// instead of using raw string literals — a stray typo ("venue-owner" vs
// "venue_owner") would silently deny or grant access.
package roles

// Role is the type for user role strings. Using this type in function
// signatures makes it explicit that only roles from this package are expected.
type Role = string

const (
	RoleUser       Role = "user"
	RoleVenueOwner Role = "venue_owner"
	RoleMaster     Role = "master"
	RoleAdmin      Role = "admin"
)

// Authenticated is the set of all registered roles. Use it with RequireRole
// when any authenticated user should be allowed through.
var Authenticated = []Role{RoleUser, RoleVenueOwner, RoleMaster, RoleAdmin}
