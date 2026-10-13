package ingest

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
