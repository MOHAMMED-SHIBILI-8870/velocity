package seed

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ProductSeed struct {
	Name        string
	Symbol      string
	Category    string
	Description string
	Price       float64
	Stock       int
}

var Products = []ProductSeed{
	{
		Name:        "Apple Inc.",
		Symbol:      "AAPL",
		Category:    "Tokenized Share",
		Description: "Tokenized equity backed 1:1 by Apple Inc. common stock.",
		Price:       185.50,
		Stock:       45,
	},
	{
		Name:        "Tesla Inc.",
		Symbol:      "TSLA",
		Category:    "Tokenized Share",
		Description: "Tokenized equity representing Tesla Inc. with instant 24/7 liquidity.",
		Price:       240.20,
		Stock:       30,
	},
	{
		Name:        "NVIDIA Corp",
		Symbol:      "NVDA",
		Category:    "Tokenized Share",
		Description: "Tokenized equity backed by NVIDIA Corp semiconductor leader.",
		Price:       128.75,
		Stock:       60,
	},
	{
		Name:        "Alphabet Google",
		Symbol:      "GOOGL",
		Category:    "Tokenized Share",
		Description: "Tokenized shares of Alphabet Inc. Class A common stock.",
		Price:       165.00,
		Stock:       25,
	},
	{
		Name:        "Velocity GPU Cloud Node",
		Symbol:      "H100-NODE",
		Category:    "Hardware / Tech",
		Description: "Dedicated 8x NVIDIA H100 SXM5 AI training instance voucher with dedicated bandwidth.",
		Price:       2500.00,
		Stock:       8,
	},
	{
		Name:        "Antminer S21 Pro Miner",
		Symbol:      "S21-PRO",
		Category:    "Hardware / Tech",
		Description: "Bitmain Antminer S21 Pro 234 TH/s Bitcoin ASIC hardware miner.",
		Price:       3800.00,
		Stock:       12,
	},
	{
		Name:        "Ledger Stax Hardware Wallet",
		Symbol:      "LEDGER-STX",
		Category:    "Hardware / Tech",
		Description: "Next-gen curved E-ink touchscreen cryptocurrency hardware signer.",
		Price:       279.00,
		Stock:       50,
	},
	{
		Name:        "Enterprise Edge Validator Rack",
		Symbol:      "VAL-RACK",
		Category:    "Hardware / Tech",
		Description: "1U rack-mount high-redundancy staking node server with dual hot-swap power supplies.",
		Price:       1450.00,
		Stock:       15,
	},
}

func SeedProducts(ctx context.Context, db *pgxpool.Pool, sellerID int64) error {
	// First ensure seller user exists
	_, err := db.Exec(ctx, `
		INSERT INTO users (id, email) 
		VALUES ($1, 'seller@example.com') 
		ON CONFLICT (id) DO NOTHING
	`, sellerID)
	if err != nil {
		log.Printf("Ensure seller user: %v", err)
	}

	// Also ensure test buyer user exists (ID 25: user1@example.com, ID 19: user)
	db.Exec(ctx, `INSERT INTO users (id, email) VALUES (25, 'user1@example.com') ON CONFLICT (id) DO NOTHING`)
	db.Exec(ctx, `INSERT INTO users (id, email) VALUES (19, 's84896329@gmail.com') ON CONFLICT (id) DO NOTHING`)

	// Ensure wallets have plenty of USDT for testing purchases
	db.Exec(ctx, `
		INSERT INTO wallets (user_id, asset, available, locked)
		VALUES ($1, 'USDT', 100000, 0)
		ON CONFLICT (user_id, asset) DO NOTHING
	`, sellerID)
	db.Exec(ctx, `
		INSERT INTO wallets (user_id, asset, available, locked)
		VALUES (25, 'USDT', 500000, 0)
		ON CONFLICT (user_id, asset) DO NOTHING
	`)
	db.Exec(ctx, `
		INSERT INTO wallets (user_id, asset, available, locked)
		VALUES (19, 'USDT', 500000, 0)
		ON CONFLICT (user_id, asset) DO NOTHING
	`)

	for _, p := range Products {
		var exists bool
		err := db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM products WHERE symbol = $1 AND seller_id = $2
			)
		`, p.Symbol, sellerID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check product %s: %w", p.Symbol, err)
		}

		if !exists {
			_, err = db.Exec(ctx, `
				INSERT INTO products (
					seller_id, name, symbol, category, description, price, stock, locked_stock, status
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, 0, 'Active'
				)
			`, sellerID, p.Name, p.Symbol, p.Category, p.Description, p.Price, p.Stock)
			if err != nil {
				return fmt.Errorf("insert product %s: %w", p.Symbol, err)
			}
			log.Printf("Seeded product: %s (%s) for seller %d\n", p.Name, p.Symbol, sellerID)
		}
	}

	return nil
}
