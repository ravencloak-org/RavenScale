package types

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrPolicyNotFound         = errors.New("acl policy not found")
	ErrPolicyUpdateIsDisabled = errors.New("update is disabled for modes other than 'database'")
)

// Policy represents a policy in the database.
type Policy struct {
	gorm.Model

	// TenantID/TailnetID scope the policy to one Tailnet (ADR-0001; one HuJSON
	// policy per Tailnet). Both never null; default to the N=1 seed (ADR-0008).
	TenantID  uint `gorm:"not null;default:1;index"`
	TailnetID uint `gorm:"not null;default:1;index"`

	// Data contains the policy in HuJSON format.
	Data string
}
