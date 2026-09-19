package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	connStr := "postgres://postgres:1234@localhost:5432/velocity?sslmode=disable"
	db, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	// 1. Deactivate old crypto symbols as requested ("not these data's")
	oldSymbols := []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"}
	for _, sym := range oldSymbols {
		_, err := db.Exec(ctx, "UPDATE symbols SET is_active = false WHERE symbol = $1", sym)
		if err != nil {
			log.Printf("error deactivating %s: %v", sym, err)
		} else {
			fmt.Printf("Deactivated old symbol: %s\n", sym)
		}
	}

	// 2. Fetch all products from products table
	rows, err := db.Query(ctx, "SELECT id, name, symbol, price FROM products ORDER BY created_at ASC")
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	defer rows.Close()

	type ProductItem struct {
		ID     string
		Name   string
		Symbol string
		Price  float64
	}
	var products []ProductItem
	for rows.Next() {
		var p ProductItem
		if err := rows.Scan(&p.ID, &p.Name, &p.Symbol, &p.Price); err != nil {
			log.Fatalf("scan product: %v", err)
		}
		products = append(products, p)
	}

	fmt.Printf("\nFound %d products to add as market symbols:\n", len(products))

	// 3. Insert each product as a market symbol paired with USDT
	for _, p := range products {
		marketSymbol := fmt.Sprintf("%sUSDT", p.Symbol)
		displayName := fmt.Sprintf("%s / USDT", p.Name)
		baseAsset := p.Symbol
		quoteAsset := "USDT"

		_, err := db.Exec(ctx, `
			INSERT INTO symbols (
				symbol, display_name, base_asset, quote_asset, 
				tick_size, lot_size, is_active, created_at
			)
			VALUES ($1, $2, $3, $4, 1, 1, true, NOW())
			ON CONFLICT (symbol) DO UPDATE 
			SET display_name = EXCLUDED.display_name,
			    base_asset = EXCLUDED.base_asset,
			    quote_asset = EXCLUDED.quote_asset,
			    is_active = true
		`, marketSymbol, displayName, baseAsset, quoteAsset)

		if err != nil {
			log.Printf("error inserting symbol %s: %v", marketSymbol, err)
		} else {
			fmt.Printf("Added/Updated Symbol: %s | Display: %s | Base: %s | Quote: %s\n", marketSymbol, displayName, baseAsset, quoteAsset)
		}
	}

	// 4. List all active symbols
	activeRows, err := db.Query(ctx, "SELECT symbol, display_name, base_asset, quote_asset, is_active FROM symbols WHERE is_active = true ORDER BY symbol ASC")
	if err != nil {
		log.Fatalf("query active symbols: %v", err)
	}
	defer activeRows.Close()

	fmt.Println("\nActive Symbols in Database:")
	for activeRows.Next() {
		var sym, disp, base, quote string
		var active bool
		activeRows.Scan(&sym, &disp, &base, &quote, &active)
		fmt.Printf("  - Symbol: %-15s | Display: %-42s | Base: %-12s | Active: %v\n", sym, disp, base, active)
	}
}
