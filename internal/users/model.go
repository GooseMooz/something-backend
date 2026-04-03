package users

import (
	"time"

	"github.com/goosemooz/something-backend/internal/types"
)

// PublicProfile is returned for unauthenticated GET /users/{id} requests.
// It omits private fields like email and phone.
type PublicProfile struct {
	ID         types.RecordID `json:"id"`
	Name       string         `json:"name"`
	Skills     []string       `json:"skills"`
	Bio        string         `json:"bio,omitempty"`
	Categories []string       `json:"categories"`
	Intensity  string         `json:"intensity"`
	XP         int            `json:"xp"`
	Instagram  string         `json:"instagram,omitempty"`
	LinkedIn   string         `json:"linkedin,omitempty"`
	S3PFP      string         `json:"s3_pfp,omitempty"`
	Badges     []string       `json:"badges"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

func (u *User) Public() PublicProfile {
	return PublicProfile{
		ID:         u.ID,
		Name:       u.Name,
		Skills:     u.Skills,
		Bio:        u.Bio,
		Categories: u.Categories,
		Intensity:  u.Intensity,
		XP:         u.XP,
		Instagram:  u.Instagram,
		LinkedIn:   u.LinkedIn,
		S3PFP:      u.S3PFP,
		Badges:     u.Badges,
		CreatedAt:  u.CreatedAt,
		UpdatedAt:  u.UpdatedAt,
	}
}

type User struct {
	ID           types.RecordID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-" cbor:"password_hash"`
	Name         string    `json:"name"`
	Skills       []string  `json:"skills"`
	Bio          string    `json:"bio,omitempty"`
	Categories   []string  `json:"categories"`
	Intensity    string    `json:"intensity"`
	Phone        string    `json:"phone,omitempty"`
	XP           int       `json:"xp"`
	Instagram    string    `json:"instagram,omitempty"`
	LinkedIn     string    `json:"linkedin,omitempty"`
	S3PFP        string    `json:"s3_pfp,omitempty"`
	S3PDF        string    `json:"s3_pdf,omitempty"`
	Badges       []string  `json:"badges"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
