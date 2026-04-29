package orgs

import (
	"time"

	"github.com/goosemooz/something-backend/internal/types"
)

type NotificationSettings struct {
	ApplicantDigest          bool   `json:"applicant_digest"`
	ApplicantDigestFrequency string `json:"applicant_digest_frequency"`
	AcceptedWithdrawal       bool   `json:"accepted_withdrawal"`
}

func DefaultNotificationSettings() NotificationSettings {
	return NotificationSettings{
		ApplicantDigest:          true,
		ApplicantDigestFrequency: "daily",
		AcceptedWithdrawal:       true,
	}
}

func NormalizeNotificationSettings(settings NotificationSettings) NotificationSettings {
	if settings.ApplicantDigestFrequency != "weekly" {
		settings.ApplicantDigestFrequency = "daily"
	}
	return settings
}

type Org struct {
	ID                    types.RecordID       `json:"id"`
	Name                  string               `json:"name"`
	PasswordHash          string               `json:"-" cbor:"password_hash"`
	Categories            []string             `json:"categories"`
	Description           string               `json:"description,omitempty"`
	Website               string               `json:"website,omitempty"`
	Email                 string               `json:"email"`
	Phone                 string               `json:"phone,omitempty"`
	Address               string               `json:"address,omitempty"`
	Location              string               `json:"location"`
	Instagram             string               `json:"instagram,omitempty"`
	LinkedIn              string               `json:"linkedin,omitempty"`
	S3PFP                 string               `json:"s3_pfp,omitempty"`
	Verified              bool                 `json:"verified"`
	NotificationSettings  NotificationSettings `json:"notification_settings"`
	ApplicantDigestSentAt *time.Time           `json:"-" cbor:"applicant_digest_sent_at"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}
