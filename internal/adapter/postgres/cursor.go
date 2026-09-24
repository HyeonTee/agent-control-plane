package postgres

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"

	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

type activityCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

type sequenceCursor struct {
	Sequence int64 `json:"sequence"`
}

func encodeCursor(value any) string {
	encoded, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeCursor(value string, dst any) error {
	if value == "" || len(value) > 512 {
		return model.ErrInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return model.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return model.ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return model.ErrInvalid
	}
	return nil
}

func parseActivityCursor(value string) (activityCursor, error) {
	if value == "" {
		return activityCursor{}, nil
	}
	var cursor activityCursor
	if err := decodeCursor(value, &cursor); err != nil {
		return activityCursor{}, err
	}
	if cursor.At.IsZero() {
		return activityCursor{}, model.ErrInvalid
	}
	if _, err := parseUUID(cursor.ID); err != nil {
		return activityCursor{}, model.ErrInvalid
	}
	return cursor, nil
}

func parseSequenceCursor(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	var cursor sequenceCursor
	if err := decodeCursor(value, &cursor); err != nil || cursor.Sequence < 1 {
		return 0, model.ErrInvalid
	}
	return cursor.Sequence, nil
}
