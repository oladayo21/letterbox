package db

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUIDToPgtype converts a google/uuid.UUID to pgtype.UUID.
func UUIDToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// PgtypeToUUID converts a pgtype.UUID to google/uuid.UUID.
func PgtypeToUUID(p pgtype.UUID) uuid.UUID {
	if !p.Valid {
		return uuid.Nil
	}

	return uuid.UUID(p.Bytes)
}
