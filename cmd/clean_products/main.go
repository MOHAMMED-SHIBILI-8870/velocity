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

	// Update stock share products to authentic seller products
	updates := []struct {
		oldSymbol   string
		newName     string
		newSymbol   string
		newCategory string
		newDesc     string
		newPrice    float64
		newStock    int
	}{
		{
			oldSymbol:   "AAPL",
			newName:     "YubiKey 5C NFC Security Key",
			newSymbol:   "YUBI-5C",
			newCategory: "Hardware / Tech",
			newDesc:     "Dual-connector USB-C and NFC FIDO2/WebAuthn hardware authentication key.",
			newPrice:    55.00,
			newStock:    60,
		},
		{
			oldSymbol:   "TSLA",
			newName:     "Starlink High Performance Kit",
			newSymbol:   "STARLINK",
			newCategory: "Hardware / Tech",
			newDesc:     "Low-latency high-throughput satellite terminal kit with dual gigabit router.",
			newPrice:    599.00,
			newStock:    15,
		},
		{
			oldSymbol:   "NVDA",
			newName:     "NVIDIA RTX 4090 Workstation Rig",
			newSymbol:   "RTX-4090",
			newCategory: "Hardware / Tech",
			newDesc:     "Liquid-cooled 24GB VRAM AI inferencing and 3D rendering workstation.",
			newPrice:    3200.00,
			newStock:    6,
		},
		{
			oldSymbol:   "GOOGL",
			newName:     "Raspberry Pi 5 Staking Cluster",
			newSymbol:   "RPI5-NODE",
			newCategory: "Hardware / Tech",
			newDesc:     "Quad-node 8GB Raspberry Pi 5 PoE cluster with 2TB NVMe PCIe carrier board.",
			newPrice:    280.00,
			newStock:    25,
		},
	}

	for _, u := range updates {
		tag, err := db.Exec(ctx, `
			UPDATE products
			SET name = $1, symbol = $2, category = $3, description = $4, price = $5, stock = $6, status = 'Active', updated_at = now()
			WHERE symbol = $7
		`, u.newName, u.newSymbol, u.newCategory, u.newDesc, u.newPrice, u.newStock, u.oldSymbol)
		if err != nil {
			log.Printf("error updating %s: %v", u.oldSymbol, err)
		} else {
			fmt.Printf("Updated %s -> %s (affected: %d)\n", u.oldSymbol, u.newSymbol, tag.RowsAffected())
		}
	}

	// Print all products
	rows, err := db.Query(ctx, "SELECT id, name, symbol, category, price, stock, status FROM products ORDER BY created_at ASC")
	if err != nil {
		log.Fatalf("query products: %v", err)
	}
	defer rows.Close()

	fmt.Println("\nCurrent Products in DB:")
	for rows.Next() {
		var id, name, symbol, cat, status string
		var price float64
		var stock int
		rows.Scan(&id, &name, &symbol, &cat, &price, &stock, &status)
		fmt.Printf("- [%s] %s (%s) | %s | $%.2f | Stock: %d | Status: %s\n", symbol, name, id, cat, price, stock, status)
	}
}
