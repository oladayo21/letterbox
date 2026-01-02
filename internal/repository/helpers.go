package repository

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func uuidToPgtype(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgtypeToUUID(pg pgtype.UUID) uuid.UUID {
	return uuid.UUID(pg.Bytes)
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func ptrInt32(i int) *int32 {
	if i == 0 {
		return nil
	}

	v := int32(i)

	return &v
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func derefInt32(i *int32) int {
	if i == nil {
		return 0
	}

	return int(*i)
}
