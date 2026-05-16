package domain

type Role string

const (
	RoleCustomer Role = "customer"
	RoleBrand    Role = "brand"
	RoleAdmin    Role = "admin"
)

func (r Role) Valid() bool {
	switch r {
	case RoleCustomer, RoleBrand, RoleAdmin:
		return true
	}
	return false
}

type UserStatus string

const (
	StatusActive  UserStatus = "active"
	StatusLocked  UserStatus = "locked"
	StatusDeleted UserStatus = "deleted"
)

type OAuthProvider string

const (
	ProviderGoogle OAuthProvider = "google"
	ProviderApple  OAuthProvider = "apple"
)

func (p OAuthProvider) Valid() bool {
	switch p {
	case ProviderGoogle, ProviderApple:
		return true
	}
	return false
}
