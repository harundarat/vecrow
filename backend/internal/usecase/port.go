// Package usecase holds the application's use cases. Each sub-package owns
// one area (auth, escrows, ...) and declares the ports (interfaces) it needs;
// implementations live in internal/repository and are wired in
// internal/bootstrap.
package usecase

import "context"

// Transactor runs fn atomically. Repository calls made with the ctx passed to
// fn join the same transaction; returning an error rolls it back.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
