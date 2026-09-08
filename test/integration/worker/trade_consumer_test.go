package worker

import (
	"context"
	"testing"
	"time"

	"velocity/internal/domain/trade"
	"velocity/internal/engine/orderbook"
	"velocity/internal/marketdata"
	"velocity/internal/persistence/postgres/generated"
	"velocity/internal/persistence/worker"
	"velocity/internal/service/settlementservice"
	"velocity/pkg/constants"
	"velocity/test/integration"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTradeConsumer_SettlementFailureRecorded(t *testing.T) {
	tc := integration.NewTestContext(t)

	service := settlementservice.New(
		tc.TxManager,
		tc.UserDispatcher,
	)

	// ---------------------------------------------------------------------
	// Symbol
	// ---------------------------------------------------------------------

	symbol := "BTCUSDT-" + uuid.NewString()[:8]

	_, err := tc.SymbolRepo.Create(
		tc.Ctx,
		generated.CreateSymbolParams{
			Symbol:      symbol,
			DisplayName: "Bitcoin",
			TickSize:    1,
			LotSize:     1,
			IsActive:    true,
		},
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------------
	// IDs
	// ---------------------------------------------------------------------

	base := time.Now().UnixNano()

	buyerID := base
	sellerID := base + 1
	buyOrderID := base + 2
	sellOrderID := base + 3
	tradeID := base + 4

	// ---------------------------------------------------------------------
	// Users
	// ---------------------------------------------------------------------

	_, err = tc.UserRepo.Create(
		tc.Ctx,
		generated.CreateUserParams{
			ID:        buyerID,
			Email:     uuid.NewString() + "@buyer.com",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	)
	require.NoError(t, err)

	_, err = tc.UserRepo.Create(
		tc.Ctx,
		generated.CreateUserParams{
			ID:        sellerID,
			Email:     uuid.NewString() + "@seller.com",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------------
	// Wallets
	//
	// Buyer deliberately has no locked USDT.
	// The trade requires 50,000 USDT, so settlement must fail.
	// ---------------------------------------------------------------------

	_, err = tc.WalletRepo.Create(
		tc.Ctx,
		generated.CreateWalletParams{
			UserID: buyerID,
			Asset:  "USDT",
			Locked: 0,
		},
	)
	require.NoError(t, err)

	_, err = tc.WalletRepo.Create(
		tc.Ctx,
		generated.CreateWalletParams{
			UserID: buyerID,
			Asset:  "BTC",
		},
	)
	require.NoError(t, err)

	_, err = tc.WalletRepo.Create(
		tc.Ctx,
		generated.CreateWalletParams{
			UserID: sellerID,
			Asset:  "BTC",
			Locked: 1,
		},
	)
	require.NoError(t, err)

	_, err = tc.WalletRepo.Create(
		tc.Ctx,
		generated.CreateWalletParams{
			UserID: sellerID,
			Asset:  "USDT",
		},
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------------
	// Orders
	// ---------------------------------------------------------------------

	price := pgtype.Int8{
		Int64: 50000,
		Valid: true,
	}

	_, err = tc.OrderRepo.Create(
		tc.Ctx,
		generated.CreateOrderParams{
			ID:          buyOrderID,
			UserID:      buyerID,
			Symbol:      symbol,
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      string(constants.OrderStatusOpen),
			Price:       price,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	)
	require.NoError(t, err)

	_, err = tc.OrderRepo.Create(
		tc.Ctx,
		generated.CreateOrderParams{
			ID:          sellOrderID,
			UserID:      sellerID,
			Symbol:      symbol,
			Side:        "SELL",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      string(constants.OrderStatusOpen),
			Price:       price,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------------
	// Consumer dependencies
	//
	// The failure path never calls DispatchTrade, so a broadcaster with
	// nil dependencies is sufficient for this specific test.
	// ---------------------------------------------------------------------

	logger := zap.NewNop()

	dispatcher := marketdata.NewBroadcaster(
		nil,
		nil,
	)

	provider := func(string) *orderbook.OrderBook {
		return nil
	}

	consumer := worker.NewTradeConsumer(
		service,
		tc.SymbolRepo,
		tc.FailedSettlementRepo,
		dispatcher,
		provider,
		logger,
	)

	// ---------------------------------------------------------------------
	// Start consumer
	// ---------------------------------------------------------------------

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trades := make(chan trade.Trade, 1)

	consumer.Start(ctx, trades)

	// ---------------------------------------------------------------------
	// Submit trade
	// ---------------------------------------------------------------------

	trades <- trade.Trade{
		ID:          tradeID,
		BuyOrderID:  buyOrderID,
		SellOrderID: sellOrderID,
		BuyerID:     buyerID,
		SellerID:    sellerID,
		Symbol:      symbol,
		Price:       50000,
		Quantity:    1,
		ExecutedAt:  time.Now(),
	}

	// ---------------------------------------------------------------------
	// Wait for TradeConsumer to record the failed settlement.
	// ---------------------------------------------------------------------

	var failed generated.FailedSettlement

	require.Eventually(
		t,
		func() bool {
			failures, err := tc.FailedSettlementRepo.ListUnresolved(
				tc.Ctx,
			)

			if err != nil {
				return false
			}

			for _, item := range failures {
				if item.TradeID == tradeID {
					failed = item
					return true
				}
			}

			return false
		},
		2*time.Second,
		10*time.Millisecond,
	)

	// ---------------------------------------------------------------------
	// Verify failed settlement record.
	// ---------------------------------------------------------------------

	require.Equal(t, tradeID, failed.TradeID)
	require.Equal(t, buyOrderID, failed.BuyOrderID)
	require.Equal(t, sellOrderID, failed.SellOrderID)
	require.Equal(t, buyerID, failed.BuyerID)
	require.Equal(t, sellerID, failed.SellerID)
	require.Equal(t, symbol, failed.Symbol)
	require.Equal(t, int64(50000), failed.Price)
	require.Equal(t, int64(1), failed.Quantity)

	require.NotEmpty(t, failed.ErrorMessage)
	require.Equal(t, int32(0), failed.RetryCount)
	require.False(t, failed.Resolved)
	require.False(t, failed.IsDead)

	// ---------------------------------------------------------------------
	// Verify settlement transaction rolled back.
	// ---------------------------------------------------------------------

	_, err = tc.TradeRepo.GetByID(
		tc.Ctx,
		tradeID,
	)
	require.Error(t, err)

	buyOrder, err := tc.OrderRepo.GetByID(
		tc.Ctx,
		buyOrderID,
	)
	require.NoError(t, err)

	require.EqualValues(t, 0, buyOrder.Filled)
	require.EqualValues(t, 1, buyOrder.Remaining)
	require.Equal(
		t,
		string(constants.OrderStatusOpen),
		buyOrder.Status,
	)

	sellOrder, err := tc.OrderRepo.GetByID(
		tc.Ctx,
		sellOrderID,
	)
	require.NoError(t, err)

	require.EqualValues(t, 0, sellOrder.Filled)
	require.EqualValues(t, 1, sellOrder.Remaining)
	require.Equal(
		t,
		string(constants.OrderStatusOpen),
		sellOrder.Status,
	)

	buyerUSDT, err := tc.WalletRepo.Get(
		tc.Ctx,
		buyerID,
		"USDT",
	)
	require.NoError(t, err)

	require.EqualValues(t, 0, buyerUSDT.Available)
	require.EqualValues(t, 0, buyerUSDT.Locked)

	sellerBTC, err := tc.WalletRepo.Get(
		tc.Ctx,
		sellerID,
		"BTC",
	)
	require.NoError(t, err)

	require.EqualValues(t, 0, sellerBTC.Available)
	require.EqualValues(t, 1, sellerBTC.Locked)
}
