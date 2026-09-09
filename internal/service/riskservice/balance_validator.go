package riskservice

import (
	"context"

	"velocity/internal/persistence/postgres/repository"
	"velocity/internal/service/walletservice"
	"velocity/pkg/constants"
	"velocity/pkg/errors"
)

type BalanceValidator struct {
	walletService *walletservice.Service
	symbolRepo    repository.SymbolRepository
}

func NewBalanceValidator(
	walletService *walletservice.Service,
	symbolRepo repository.SymbolRepository,
) *BalanceValidator {
	return &BalanceValidator{
		walletService: walletService,
		symbolRepo:    symbolRepo,
	}
}

func (v *BalanceValidator) Validate(
	ctx context.Context,
	req ValidateOrderRequest,
) error {

	o := req.Order
	userID := o.UserID

	symbol, err := v.symbolRepo.Get(ctx, o.Symbol)
	if err != nil {
		return err
	}

	if o.Side == constants.OrderSideBuy {
		wallet, err := v.walletService.Get(
			ctx,
			userID,
			symbol.QuoteAsset,
		)
		if err != nil {
			return errors.ErrInsufficientBalance
		}

		required := o.Price * o.Quantity
		if wallet.Available < required {
			return errors.ErrInsufficientBalance
		}
	} else if o.Side == constants.OrderSideSell {
		wallet, err := v.walletService.Get(
			ctx,
			userID,
			symbol.BaseAsset,
		)
		if err != nil {
			return errors.ErrInsufficientBalance
		}

		if wallet.Available < o.Quantity {
			return errors.ErrInsufficientBalance
		}
	}

	return nil
}
