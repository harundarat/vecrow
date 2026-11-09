package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
)

type fakeViewer struct {
	perm      nearrpc.Permission
	keyErr    error
	exists    bool
	existsErr error
}

func (f fakeViewer) ViewAccessKey(context.Context, string, string) (nearrpc.Permission, error) {
	return f.perm, f.keyErr
}

func (f fakeViewer) AccountExists(context.Context, string) (bool, error) {
	return f.exists, f.existsErr
}

func TestNEARAccessKeys_Status(t *testing.T) {
	down := errors.New("timeout")
	tests := []struct {
		name    string
		viewer  fakeViewer
		want    auth.AccessKeyStatus
		wantErr error
	}{
		{name: "full access", viewer: fakeViewer{perm: nearrpc.FullAccess}, want: auth.AccessKeyFullAccess},
		{name: "function call", viewer: fakeViewer{perm: nearrpc.FunctionCall}, want: auth.AccessKeyLimited},
		{name: "unknown account", viewer: fakeViewer{keyErr: nearrpc.ErrUnknownAccount}, want: auth.AccountMissing},
		{name: "unknown key on existing account", viewer: fakeViewer{keyErr: nearrpc.ErrUnknownAccessKey, exists: true}, want: auth.AccessKeyMissing},
		{name: "unknown key, account missing", viewer: fakeViewer{keyErr: nearrpc.ErrUnknownAccessKey}, want: auth.AccountMissing},
		{name: "rpc error", viewer: fakeViewer{keyErr: down}, wantErr: down},
		{name: "rpc error on account lookup", viewer: fakeViewer{keyErr: nearrpc.ErrUnknownAccessKey, existsErr: down}, wantErr: down},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repository.NewNEARAccessKeys(tt.viewer).Status(context.Background(), "alice.testnet", "ed25519:x")
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Fatalf("Status = %v, %v; want %v, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
