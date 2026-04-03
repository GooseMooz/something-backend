package types

import (
	"encoding/json"

	"github.com/surrealdb/surrealdb.go/pkg/models"
)

// RecordID wraps the library's models.RecordID to handle CBOR deserialization
// (via promoted UnmarshalCBOR) while serializing to a plain "table:id" string in JSON.
type RecordID struct {
	models.RecordID
}

func (r RecordID) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.RecordID.String())
}

func (r *RecordID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := models.ParseRecordID(s)
	if err != nil {
		return err
	}
	r.RecordID = *parsed
	return nil
}

func (r RecordID) String() string {
	return r.RecordID.String()
}
