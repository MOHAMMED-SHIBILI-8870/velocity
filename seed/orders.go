package seed

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderSeedItem struct {
	ID          int64
	UserID      int64
	Symbol      string
	Side        string
	OrderType   string
	TimeInForce string
	Status      string
	Price       int64
	StopPrice   int64
	Quantity    int64
	Remaining   int64
	Filled      int64
	CreatedAt   time.Time
}

func SeedOrders(
	ctx context.Context,
	db *pgxpool.Pool,
) error {
	now := time.Now().UTC()

	orders := []OrderSeedItem{
		// User 19 (shibili) - Open Orders
		{
			ID:          3556001001,
			UserID:      19,
			Symbol:      "BTCUSDT",
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "OPEN",
			Price:       59500,
			StopPrice:   0,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   now.Add(-2 * time.Hour),
		},
		{
			ID:          3556001002,
			UserID:      19,
			Symbol:      "BTCUSDT",
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "OPEN",
			Price:       56000,
			StopPrice:   0,
			Quantity:    2,
			Remaining:   2,
			Filled:      0,
			CreatedAt:   now.Add(-45 * time.Minute),
		},
		// User 19 (shibili) - History Orders
		{
			ID:          3556001003,
			UserID:      19,
			Symbol:      "BTCUSDT",
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "FILLED",
			Price:       61200,
			StopPrice:   0,
			Quantity:    1,
			Remaining:   0,
			Filled:      1,
			CreatedAt:   now.Add(-24 * time.Hour),
		},
		{
			ID:          3556001004,
			UserID:      19,
			Symbol:      "BTCUSDT",
			Side:        "SELL",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "CANCELLED",
			Price:       68500,
			StopPrice:   0,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   now.Add(-12 * time.Hour),
		},

		// User 1
		{
			ID:          3556001005,
			UserID:      1,
			Symbol:      "BTCUSDT",
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "OPEN",
			Price:       57500,
			StopPrice:   0,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   now.Add(-3 * time.Hour),
		},

		// User 25
		{
			ID:          3556001006,
			UserID:      25,
			Symbol:      "BTCUSDT",
			Side:        "BUY",
			OrderType:   "LIMIT",
			TimeInForce: "GTC",
			Status:      "OPEN",
			Price:       58200,
			StopPrice:   0,
			Quantity:    1,
			Remaining:   1,
			Filled:      0,
			CreatedAt:   now.Add(-1 * time.Hour),
		},
	}

	for _, o := range orders {
		_, err := db.Exec(
			ctx,
			`INSERT INTO orders (
				id, user_id, symbol, side, order_type, time_in_force, status,
				price, stop_price, quantity, remaining, filled, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (id) DO NOTHING`,
			o.ID, o.UserID, o.Symbol, o.Side, o.OrderType, o.TimeInForce, o.Status,
			o.Price, o.StopPrice, o.Quantity, o.Remaining, o.Filled, o.CreatedAt, o.CreatedAt,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
