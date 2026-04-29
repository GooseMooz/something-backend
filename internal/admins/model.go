package admins

import (
	"time"

	"github.com/goosemooz/something-backend/internal/types"
)

type Admin struct {
	ID           types.RecordID `json:"id"`
	Email        string         `json:"email"`
	PasswordHash string         `json:"-" cbor:"password_hash"`
	Name         string         `json:"name"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
