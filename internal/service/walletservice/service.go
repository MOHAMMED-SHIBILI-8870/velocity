package walletservice

import (
	"context"
	stderrors "errors"
	"velocity/internal/persistence/postgres/generated"
	"velocity/internal/persistence/postgres/repository"
	"velocity/pkg/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Service struct {
	walletRepo      repository.WalletRepository
	transactionRepo repository.WalletTransactionRepository
}

func New(
	walletRepo repository.WalletRepository,
	transactionRepo repository.WalletTransactionRepository,
) *Service {
	return &Service{
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
	}
}

func (s *Service) getOrCreateForUpdate(
	ctx context.Context,
	userID int64,
	asset string,
) (generated.Wallet, error) {
	wallet, err := s.walletRepo.GetForUpdate(ctx, userID, asset)
	if err == nil {
		return wallet, nil
	}
	if stderrors.Is(err, pgx.ErrNoRows) {
		wallet, err = s.walletRepo.Create(
			ctx,
			generated.CreateWalletParams{
				UserID:    userID,
				Asset:     asset,
				Available: 0,
				Locked:    0,
			},
		)
		if err != nil {
			return s.walletRepo.GetForUpdate(ctx, userID, asset)
		}
		return wallet, nil
	}
	return generated.Wallet{}, err
}

func (s *Service) Get(ctx context.Context, userID int64, asset string) (generated.Wallet, error) {
	return s.walletRepo.Get(ctx, userID, asset)
}

func (s *Service) List(ctx context.Context, userID int64) ([]generated.Wallet, error) {
	return s.walletRepo.List(ctx, userID)
}

func (s *Service) Create(ctx context.Context, params generated.CreateWalletParams) (generated.Wallet, error) {
	return s.walletRepo.Create(ctx, params)
}

func (s *Service) Update(ctx context.Context, params generated.UpdateWalletParams) error {
	return s.walletRepo.Update(ctx, params)
}

func (s *Service) LockFunds(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
) error {

	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.Get(ctx, userID, asset)
	if err != nil {
		return errors.ErrInsufficientBalance
	}

	return s.walletRepo.LockFunds(
		ctx,
		wallet.ID,
		amount,
	)
}

func (s *Service) UnlockFunds(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
) error {

	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.Get(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	if wallet.Locked < amount {
		return errors.ErrInsufficientLockedBalance
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID: wallet.ID,

			Available: wallet.Available + amount,
			Locked:    wallet.Locked - amount,
		},
	)
}

func (s *Service) Deposit(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.getOrCreateForUpdate(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  asset,
			Amount: amount,
			Type:   "DEPOSIT",
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available + amount,
			Locked:    wallet.Locked,
		},
	)
}

func (s *Service) Withdraw(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.GetForUpdate(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	if wallet.Available < amount {
		return errors.ErrInsufficientBalance
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  asset,
			Amount: amount,
			Type:   "WITHDRAWAL",
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available - amount,
			Locked:    wallet.Locked,
		},
	)
}

func (s *Service) Convert(
	ctx context.Context,
	userID int64,
	fromAsset string,
	toAsset string,
	amount int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}
	if fromAsset == toAsset {
		return errors.New(errors.CodeValidation, "cannot convert between identical assets")
	}

	fromWallet, err := s.walletRepo.GetForUpdate(
		ctx,
		userID,
		fromAsset,
	)
	if err != nil {
		return err
	}

	if fromWallet.Available < amount {
		return errors.ErrInsufficientBalance
	}

	toWallet, err := s.GetOrCreateWallet(
		ctx,
		userID,
		toAsset,
	)
	if err != nil {
		return err
	}

	// 1. Deduct from source wallet
	err = s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        fromWallet.ID,
			Available: fromWallet.Available - amount,
			Locked:    fromWallet.Locked,
		},
	)
	if err != nil {
		return err
	}

	// 2. Credit destination wallet
	err = s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        toWallet.ID,
			Available: toWallet.Available + amount,
			Locked:    toWallet.Locked,
		},
	)
	if err != nil {
		return err
	}

	// 3. Record transaction logs for both sides
	_, _ = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  fromAsset,
			Amount: amount,
			Type:   "CONVERT_OUT",
		},
	)

	_, _ = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  toAsset,
			Amount: amount,
			Type:   "CONVERT_IN",
		},
	)

	return nil
}

func (s *Service) ConsumeLockedFunds(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.GetForUpdate(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	if wallet.Locked < amount {
		return errors.ErrInsufficientLockedBalance
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available,
			Locked:    wallet.Locked - amount,
		},
	)
}

func (s *Service) GetWalletByAsset(
	ctx context.Context,
	userID int64,
	asset string,
) (generated.Wallet, error) {

	return s.walletRepo.Get(
		ctx,
		userID,
		asset,
	)
}

func (s *Service) ListTransactions(
	ctx context.Context,
	userID int64,
) ([]generated.WalletTransaction, error) {
	return s.transactionRepo.ListByUser(ctx, userID)
}

func (s *Service) ListTransactionsByAsset(
	ctx context.Context,
	userID int64,
	asset string,
) ([]generated.WalletTransaction, error) {
	return s.transactionRepo.ListByUserAndAsset(
		ctx,
		generated.ListWalletTransactionsByUserAndAssetParams{
			UserID: userID,
			Asset:  asset,
		},
	)
}

func (s *Service) CreditFromTrade(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
	tradeID int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.GetForUpdate(ctx, userID, asset)
	if err != nil {
		return err
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID:  userID,
			Asset:   asset,
			Amount:  amount,
			Type:    "TRADE_CREDIT",
			TradeID: pgtype.Int8{Int64: tradeID, Valid: true},
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available + amount,
			Locked:    wallet.Locked,
		},
	)
}

func (s *Service) DebitFromTrade(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
	tradeID int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.GetForUpdate(ctx, userID, asset)
	if err != nil {
		return err
	}

	if wallet.Available < amount {
		return errors.ErrInsufficientBalance
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID:  userID,
			Asset:   asset,
			Amount:  amount,
			Type:    "TRADE_DEBIT",
			TradeID: pgtype.Int8{Int64: tradeID, Valid: true},
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available - amount,
			Locked:    wallet.Locked,
		},
	)
}

func (s *Service) DepositFromTrade(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
	tradeID int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.getOrCreateForUpdate(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  asset,
			Amount: amount,
			Type:   "TRADE_CREDIT",
			TradeID: pgtype.Int8{
				Int64: tradeID,
				Valid: true,
			},
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available + amount,
			Locked:    wallet.Locked,
		},
	)
}

func (s *Service) ConsumeLockedFundsFromTrade(
	ctx context.Context,
	userID int64,
	asset string,
	amount int64,
	tradeID int64,
) error {
	if amount <= 0 {
		return errors.ErrInvalidQuantity
	}

	wallet, err := s.walletRepo.GetForUpdate(
		ctx,
		userID,
		asset,
	)
	if err != nil {
		return err
	}

	if wallet.Locked < amount {
		return errors.ErrInsufficientLockedBalance
	}

	_, err = s.transactionRepo.Create(
		ctx,
		generated.CreateWalletTransactionParams{
			UserID: userID,
			Asset:  asset,
			Amount: amount,
			Type:   "TRADE_DEBIT",
			TradeID: pgtype.Int8{
				Int64: tradeID,
				Valid: true,
			},
		},
	)
	if err != nil {
		return err
	}

	return s.walletRepo.Update(
		ctx,
		generated.UpdateWalletParams{
			ID:        wallet.ID,
			Available: wallet.Available,
			Locked:    wallet.Locked - amount,
		},
	)
}
