package model

import "context"

// Transactor runs fn atomically; implemented by database.DB.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
