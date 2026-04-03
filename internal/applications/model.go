package applications

import (
	"time"

	"github.com/goosemooz/something-backend/internal/types"
	"github.com/goosemooz/something-backend/internal/users"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
)

type Application struct {
	ID            types.RecordID `json:"id"`
	UserID        types.RecordID `json:"user_id"`
	OpportunityID types.RecordID `json:"opportunity_id"`
	Status        Status         `json:"status"`
	XPAwarded     bool           `json:"xp_awarded"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type OrgApplication struct {
	Application
	User        *users.User         `json:"user,omitempty"`
	Opportunity *OpportunitySummary `json:"opportunity,omitempty"`
}

type OpportunitySummary struct {
	ID         types.RecordID `json:"id"`
	OrgID      types.RecordID `json:"org_id"`
	Title      string         `json:"title"`
	Category   string         `json:"category"`
	Difficulty int            `json:"difficulty"`
	Date       time.Time      `json:"date"`
	Duration   float64        `json:"duration"`
	Location   string         `json:"location"`
	MaxSpots   int            `json:"max_spots"`
	SpotsLeft  int            `json:"spots_left"`
}
