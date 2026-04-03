package opportunities

import (
	"time"

	"github.com/goosemooz/something-backend/internal/types"
)

type Difficulty int

const (
	DifficultyEasy   Difficulty = iota // 0
	DifficultyMedium                   // 1
	DifficultyHard                     // 2
)

type Opportunity struct {
	ID            types.RecordID `json:"id"`
	OrgID         types.RecordID `json:"org_id"`
	Title         string         `json:"title"`
	Category      string         `json:"category"`
	Difficulty    Difficulty     `json:"difficulty"`
	Description   string         `json:"description"`
	Date          time.Time      `json:"date"`
	Duration      float64        `json:"duration"`
	Location      string         `json:"location"`
	MaxSpots      int            `json:"max_spots"`
	SpotsLeft     int            `json:"spots_left"`
	Recurring     string         `json:"recurring,omitempty"`
	DropIn        bool           `json:"drop_in"`
	EventLink     string         `json:"event_link,omitempty"`
	ResourcesLink string         `json:"resources_link,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}
