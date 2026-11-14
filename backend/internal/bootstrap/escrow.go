package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/harundarat/vecrow/backend/internal/config"
	"github.com/harundarat/vecrow/backend/internal/domain"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/cache"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/database"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/nearrpc"
	"github.com/harundarat/vecrow/backend/internal/infrastructure/oneclick"
	"github.com/harundarat/vecrow/backend/internal/neartx"
	"github.com/harundarat/vecrow/backend/internal/repository"
	"github.com/harundarat/vecrow/backend/internal/usecase/auth"
	"github.com/harundarat/vecrow/backend/internal/usecase/escrow"
	"github.com/harundarat/vecrow/backend/internal/usecase/funding"
)

const (
	// nearRPCTimeout bounds each NEAR RPC view call.
	nearRPCTimeout = 10 * time.Second
	// nearSendTimeout bounds a send_tx call, which waits until the
	// transaction is final; a timeout is followed by status lookups.
	nearSendTimeout = 30 * time.Second
	// nearAwaitTimeout bounds how long a transaction whose send timed out
	// is looked up before its outcome is reported unknown.
	nearAwaitTimeout = 2 * time.Minute
	// oneClickTimeout bounds each 1Click call.
	oneClickTimeout = 30 * time.Second
)

// escrowStack is the escrow and funding use cases, shared by the api and
// the worker.
type escrowStack struct {
	escrows *escrow.Service
	funding *funding.Service
	guests  *auth.GuestTokenService
	keys    *repository.NEARAccessKeys
	nonces  *repository.NonceStore
}

// newEscrowStack wires the escrow and funding use cases. withStorage adds the
// storage account that registers payout recipients; without it, storage
// registration fails (the api never registers storage; the worker does).
func newEscrowStack(cfg config.Config, db *database.Postgres, kv cache.Store, log *slog.Logger, withStorage bool) (*escrowStack, error) {
	if err := cfg.RequireEscrow(withStorage); err != nil {
		return nil, err
	}
	view := nearrpc.New(cfg.NEAR.RPCURL, nearRPCTimeout)
	send := nearrpc.New(cfg.NEAR.RPCURL, nearSendTimeout)
	nonces := nearrpc.NewCacheNonces(kv)

	relayerKey, err := neartx.ParsePrivateKey(cfg.Relayer.PrivateKey)
	if err != nil {
		// The parse error never contains the key.
		return nil, fmt.Errorf("RELAYER_PRIVATE_KEY: %w", err)
	}
	relayer := nearrpc.NewSigner(send, cfg.Relayer.AccountID, relayerKey, nonces,
		nearrpc.WithAwaitTimeout(nearAwaitTimeout))
	contract := repository.NewEscrowContract(cfg.Escrow.ContractID, view, relayer)

	var storage escrow.Storage = noStorage{}
	if withStorage {
		storageKey, err := neartx.ParsePrivateKey(cfg.Storage.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("STORAGE_PRIVATE_KEY: %w", err)
		}
		payer := nearrpc.NewSigner(send, cfg.Storage.AccountID, storageKey, nonces,
			nearrpc.WithAwaitTimeout(nearAwaitTimeout))
		storage = repository.NewFTStorage(cfg.Escrow.USDCContractID, view, payer)
	}

	accounts := repository.NewAccountRepository(db)
	keys := repository.NewNEARAccessKeys(view)
	guests := auth.NewGuestTokenService(repository.NewGuestTokenStore(kv), cfg.Auth.GuestTokenTTL, auth.WithLogger(log))

	escrows := escrow.NewService(escrow.Deps{
		Repo:     repository.NewEscrowRepository(db),
		Tx:       db,
		Wallets:  accounts,
		Invitees: accounts,
		Keys:     keys,
		Contract: contract,
		Storage:  storage,
		Guests:   guests,
	}, escrow.Config{
		FeeBPS:        *cfg.Escrow.FeeBPS,
		TermsTTL:      cfg.Escrow.TermsTTL,
		AppURL:        cfg.Escrow.AppURL,
		RelayerMaxGas: cfg.Relayer.MaxGasTGas * neartx.TGas,
		ExpiryGrace:   cfg.Escrow.ExpiryGrace,
	}, escrow.WithLogger(log))

	oc := oneclick.New(cfg.OneClick.URL, cfg.OneClick.APIKey, &http.Client{Timeout: oneClickTimeout})
	fundingService := funding.NewService(escrows, accounts, repository.NewFundingRepository(db),
		repository.NewOneClickGateway(oc), funding.Config{
			USDCAssetID:  "nep141:" + cfg.Escrow.USDCContractID,
			SlippageBPS:  cfg.OneClick.SlippageBPS,
			QuoteTTL:     cfg.OneClick.QuoteTTL,
			WatchGrace:   cfg.OneClick.WatchGrace,
			RefreshAfter: cfg.OneClick.RefreshAfter,
			TokensTTL:    cfg.OneClick.TokensTTL,
		}, funding.WithLogger(log))

	return &escrowStack{
		escrows: escrows,
		funding: fundingService,
		guests:  guests,
		keys:    keys,
		nonces:  repository.NewNonceStore(kv),
	}, nil
}

// errNoStorage is returned when the api would register storage; only the
// worker holds the storage account.
var errNoStorage = errors.New("storage registration runs in the worker")

type noStorage struct{}

func (noStorage) IsRegistered(context.Context, string) (bool, error) { return false, errNoStorage }

func (noStorage) Register(context.Context, string, string) (domain.Transaction, error) {
	return domain.Transaction{}, errNoStorage
}
