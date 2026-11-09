package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

// NEARViewer is the part of nearrpc.Client used here.
type NEARViewer interface {
	ViewAccessKey(ctx context.Context, accountID, publicKey string) (nearrpc.Permission, error)
	AccountExists(ctx context.Context, accountID string) (bool, error)
}

// NEARAccessKeys reports how a public key relates to a NEAR account.
type NEARAccessKeys struct {
	rpc NEARViewer
}

var _ auth.NEARAccessKeys = (*NEARAccessKeys)(nil)

func NewNEARAccessKeys(rpc NEARViewer) *NEARAccessKeys {
	return &NEARAccessKeys{rpc: rpc}
}

func (k *NEARAccessKeys) Status(ctx context.Context, accountID, publicKey string) (auth.AccessKeyStatus, error) {
	perm, err := k.rpc.ViewAccessKey(ctx, accountID, publicKey)
	switch {
	case errors.Is(err, nearrpc.ErrUnknownAccount):
		return auth.AccountMissing, nil
	case errors.Is(err, nearrpc.ErrUnknownAccessKey):
		// NEAR answers "unknown access key" for missing accounts too, so ask
		// for the account explicitly.
		exists, err := k.rpc.AccountExists(ctx, accountID)
		if err != nil {
			return 0, fmt.Errorf("near access keys: %w", err)
		}
		if !exists {
			return auth.AccountMissing, nil
		}
		return auth.AccessKeyMissing, nil
	case err != nil:
		return 0, fmt.Errorf("near access keys: %w", err)
	case perm == nearrpc.FullAccess:
		return auth.AccessKeyFullAccess, nil
	default:
		return auth.AccessKeyLimited, nil
	}
}
