package main

import (
	"context"
	"fmt"
	"log"
	"math"

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

var catalog = []ProductSeed{
	{
		Name:        "catrige",
		Symbol:      "CTG",
		Category:    "Hardware / Tech",
		Description: "High-density multi-gigabit optical fiber transceiver module for enterprise networks.",
		Price:       100.25,
		Stock:       45,
	},
	{
		Name:        "Enterprise Edge Validator Rack",
		Symbol:      "VAL-RACK",
		Category:    "Hardware / Tech",
		Description: "1U rack-mount high-redundancy staking node server with dual hot-swap power supplies.",
		Price:       1450.00,
		Stock:       15,
	},
	{
		Name:        "Ledger Stax Hardware Wallet",
		Symbol:      "LEDGER-STX",
		Category:    "Hardware / Tech",
		Description: "Next-gen curved E-ink touchscreen cryptocurrency hardware signer with Bluetooth.",
		Price:       277.69,
		Stock:       50,
	},
	{
		Name:        "Antminer S21 Pro Miner",
		Symbol:      "S21-PRO",
		Category:    "Hardware / Tech",
		Description: "Bitmain Antminer S21 Pro 234 TH/s Bitcoin ASIC hardware miner with 15.0 J/TH efficiency.",
		Price:       3800.00,
		Stock:       12,
	},
	{
		Name:        "Velocity GPU Cloud Node",
		Symbol:      "H100-NODE",
		Category:    "Hardware / Tech",
		Description: "Dedicated 8x NVIDIA H100 SXM5 AI training instance voucher with dedicated 3.2Tbps fabric.",
		Price:       2500.00,
		Stock:       8,
	},
	{
		Name:        "Raspberry Pi 5 Staking Cluster",
		Symbol:      "RPI5-NODE",
		Category:    "Hardware / Tech",
		Description: "Quad-node 8GB Raspberry Pi 5 PoS cluster with 2TB NVMe PCIe carrier board.",
		Price:       280.00,
		Stock:       35,
	},
	{
		Name:        "NVIDIA RTX 4090 Workstation Rig",
		Symbol:      "RTX-4090",
		Category:    "Hardware / Tech",
		Description: "Liquid-cooled 24GB VRAM AI inferencing and 3D rendering workstation.",
		Price:       3200.00,
		Stock:       10,
	},
	{
		Name:        "Starlink High Performance Kit",
		Symbol:      "STARLINK",
		Category:    "Hardware / Tech",
		Description: "Low-latency high-throughput satellite terminal kit with dual gigabit router.",
		Price:       599.00,
		Stock:       20,
	},
	{
		Name:        "YubiKey 5C NFC Security Key",
		Symbol:      "YUBI-5C",
		Category:    "Hardware / Tech",
		Description: "FIDO2 / WebAuthn dual-connector USB-C & NFC hardware security key.",
		Price:       55.00,
		Stock:       120,
	},
	{
		Name:        "NVIDIA H200 Tensor Core Node",
		Symbol:      "H200-SXM",
		Category:    "Hardware / Tech",
		Description: "Next-gen 141GB HBM3e AI supercomputing node with 4.8TB/s memory bandwidth.",
		Price:       4200.00,
		Stock:       6,
	},
	{
		Name:        "Trezor Safe 5 Hardware Signer",
		Symbol:      "TREZOR-S5",
		Category:    "Hardware / Tech",
		Description: "Color touchscreen crypto signer with NDA-free certified EAL6+ secure element.",
		Price:       169.00,
		Stock:       60,
	},
	{
		Name:        "Apple Vision Pro Dev Kit",
		Symbol:      "APPL-VP",
		Category:    "Hardware / Tech",
		Description: "Spatial computing developer system with dual 4K micro-OLED displays and M2+R1 chips.",
		Price:       3499.00,
		Stock:       5,
	},
	{
		Name:        "Dell PowerEdge R760 Server",
		Symbol:      "DELL-R760",
		Category:    "Hardware / Tech",
		Description: "2U dual 4th Gen Intel Xeon Scalable enterprise virtualization rack server.",
		Price:       4850.00,
		Stock:       7,
	},
	{
		Name:        "Antminer L7 9050M Scrypt Miner",
		Symbol:      "L7-MINER",
		Category:    "Hardware / Tech",
		Description: "Bitmain Antminer L7 9050 MH/s LTC/DOGE dual-mining hardware with 3260W PSU.",
		Price:       4100.00,
		Stock:       14,
	},
	{
		Name:        "Bobcat Miner 300 Helium Gateway",
		Symbol:      "BOBCAT-300",
		Category:    "Hardware / Tech",
		Description: "High-efficiency LoRaWAN IoT gateway hotspot for Helium decentralized wireless network.",
		Price:       199.00,
		Stock:       40,
	},
	{
		Name:        "Ubiquiti UniFi Cloud Gateway Max",
		Symbol:      "UBI-UCG",
		Category:    "Hardware / Tech",
		Description: "Compact 2.5 Gbps multi-WAN enterprise gateway router with NVMe storage bay.",
		Price:       279.00,
		Stock:       30,
	},
	{
		Name:        "Tesla Powerwall 3 Green Share",
		Symbol:      "PWALL-3",
		Category:    "Tokenized Share",
		Description: "Tokenized asset share of an operational Tesla Powerwall 3 solar-plus-storage virtual power plant.",
		Price:       1850.00,
		Stock:       25,
	},
	{
		Name:        "Starlink Mini Portable Kit",
		Symbol:      "STAR-MINI",
		Category:    "Hardware / Tech",
		Description: "Ultra-portable backpack-sized satellite broadband kit with built-in Wi-Fi router.",
		Price:       399.00,
		Stock:       25,
	},
	{
		Name:        "Apple Inc. (AAPL) Tokenized Share",
		Symbol:      "APPLE-SHR",
		Category:    "Tokenized Share",
		Description: "100% collateralized tokenized fractional equity share of Apple Inc. (NASDAQ: AAPL).",
		Price:       225.50,
		Stock:       100,
	},
	{
		Name:        "NVIDIA Corp (NVDA) Tokenized Share",
		Symbol:      "NVDA-SHR",
		Category:    "Tokenized Share",
		Description: "Fully backed tokenized equity share of NVIDIA Corporation AI accelerator leader.",
		Price:       124.80,
		Stock:       150,
	},
}

func main() {
	ctx := context.Background()

	// 1. Update identity service DB (velocity_dashboard)
	idConnStr := "postgres://postgres:1234@localhost:5432/velocity_dashboard?sslmode=disable"
	idDb, err := pgxpool.New(ctx, idConnStr)
	if err != nil {
		log.Fatalf("connect identity db: %v", err)
	}
	defer idDb.Close()

	// Realistic user definitions
	seedProfiles := []struct {
		email    string
		fullName string
		role     string
	}{
		{"admin@example.com", "Arthur Pendelton", "admin"},
		{"seller@example.com", "Nexus Hardware Solutions", "seller"},
		{"seller1@example.com", "Apex Mining & Staking", "seller"},
		{"seller2@example.com", "Vanguard Enterprise Systems", "seller"},
		{"seller3@example.com", "Cipher Global Vaults", "seller"},
		{"user1@example.com", "Alexander Vance", "user"},
		{"user2@example.com", "Elena Rostova", "user"},
		{"user3@example.com", "Marcus Chen", "user"},
	}

	for _, p := range seedProfiles {
		_, _ = idDb.Exec(ctx, `
			UPDATE users 
			SET full_name = $1, role = $2 
			WHERE email = $3
		`, p.fullName, p.role, p.email)
	}
	// Also update any previous existing user IDs
	_, _ = idDb.Exec(ctx, "UPDATE users SET full_name = 'Devon Miller' WHERE id = 19 OR email = 's84896329@gmail.com'")
	_, _ = idDb.Exec(ctx, "UPDATE users SET full_name = 'Sophia Patel' WHERE id = 25 OR email = 'sophia.patel@velocity.dev'")
	fmt.Println("Updated identity-service user profiles with realistic full names and roles.")

	// Fetch seller IDs from identity DB
	sellerMap := make(map[string]int64)
	sRows, err := idDb.Query(ctx, "SELECT id, email FROM users WHERE role = 'seller' ORDER BY id ASC")
	if err == nil {
		for sRows.Next() {
			var sid int64
			var semail string
			_ = sRows.Scan(&sid, &semail)
			sellerMap[semail] = sid
			fmt.Printf("Detected Seller in DB: ID=%d, Email=%s\n", sid, semail)
		}
		sRows.Close()
	}

	// Fetch regular user IDs from identity DB
	var regularUserIDs []int64
	uRows, err := idDb.Query(ctx, "SELECT id FROM users WHERE role = 'user' ORDER BY id ASC")
	if err == nil {
		for uRows.Next() {
			var uid int64
			_ = uRows.Scan(&uid)
			regularUserIDs = append(regularUserIDs, uid)
		}
		uRows.Close()
	}
	// Ensure default regular user IDs are present
	for _, defaultUID := range []int64{1, 2, 3, 19, 25} {
		found := false
		for _, u := range regularUserIDs {
			if u == defaultUID {
				found = true
				break
			}
		}
		if !found {
			regularUserIDs = append(regularUserIDs, defaultUID)
		}
	}

	// Fallback seller IDs if none queried
	sellerList := []int64{}
	for _, semail := range []string{"seller@example.com", "seller1@example.com", "seller2@example.com", "seller3@example.com"} {
		if id, ok := sellerMap[semail]; ok {
			sellerList = append(sellerList, id)
		}
	}
	if len(sellerList) == 0 {
		sellerList = []int64{21, 22, 23, 24}
	}

	// 2. Connect to Velocity Core DB
	connStr := "postgres://postgres:1234@localhost:5432/velocity?sslmode=disable"
	db, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("connect velocity db: %v", err)
	}
	defer db.Close()

	fmt.Println("=== Starting Velocity System Reset and Seeding ===")

	// A. Clear stuck orders
	_, err = db.Exec(ctx, "DELETE FROM orders WHERE status = 'OPEN'")
	if err != nil {
		fmt.Printf("Notice on clear orders: %v\n", err)
	} else {
		fmt.Println("Cleared stale OPEN trading orders.")
	}

	// B. Deduplicate symbols table
	_, _ = db.Exec(ctx, "DELETE FROM symbols a USING symbols b WHERE a.ctid < b.ctid AND a.symbol = b.symbol")
	fmt.Println("Cleaned duplicate entries in symbols table.")

	// C. Synchronize users into velocity DB
	for semail, sid := range sellerMap {
		_, _ = db.Exec(ctx, `
			INSERT INTO users (id, email, created_at, updated_at)
			VALUES ($1, $2, now(), now())
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
		`, sid, semail)
	}
	for _, uid := range regularUserIDs {
		_, _ = db.Exec(ctx, `
			INSERT INTO users (id, email, created_at, updated_at)
			VALUES ($1, $2, now(), now())
			ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
		`, uid, fmt.Sprintf("user%d@velocity.dev", uid))
	}

	// D. Reset Wallets:
	// "don't put the fund into the seller only the role.user"
	// Regular users get 50,000 USDT available, 0 locked.
	for _, uid := range regularUserIDs {
		_, err = db.Exec(ctx, `
			INSERT INTO wallets (user_id, asset, available, locked, updated_at)
			VALUES ($1, 'USDT', 50000, 0, now())
			ON CONFLICT (user_id, asset) DO UPDATE SET available = 50000, locked = 0, updated_at = now()
		`, uid)
		if err != nil {
			log.Printf("fund user %d wallet: %v", uid, err)
		}
		_, _ = db.Exec(ctx, "UPDATE wallets SET locked = 0 WHERE user_id = $1", uid)
	}
	fmt.Println("Wallets funded: Regular users have 50,000 USDT available balance and 0 locked balance.")

	// Ensure sellers have NO USDT funds (available = 0, locked = 0)
	for _, sid := range sellerList {
		_, _ = db.Exec(ctx, `
			INSERT INTO wallets (user_id, asset, available, locked, updated_at)
			VALUES ($1, 'USDT', 0, 0, now())
			ON CONFLICT (user_id, asset) DO UPDATE SET available = 0, locked = 0, updated_at = now()
		`, sid)
	}
	fmt.Println("Seller wallets set to 0 USDT balance (sellers only hold product stock).")

	// E. Distribute 20 Products among the 4 sellers (5 products each)
	fmt.Printf("Distributing %d products across %d sellers...\n", len(catalog), len(sellerList))
	productIDMap := make(map[string]string)
	sellerProductCount := make(map[int64]int)

	for idx, p := range catalog {
		sellerIdx := idx % len(sellerList)
		assignedSellerID := sellerList[sellerIdx]

		var prodID string
		// Try to find existing product by symbol
		_ = db.QueryRow(ctx, "SELECT id FROM products WHERE symbol = $1 LIMIT 1", p.Symbol).Scan(&prodID)

		if prodID == "" {
			err := db.QueryRow(ctx, `
				INSERT INTO products (seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 'Active', now(), now())
				RETURNING id
			`, assignedSellerID, p.Name, p.Symbol, p.Category, p.Description, p.Price, p.Stock).Scan(&prodID)
			if err != nil {
				fmt.Printf("Error inserting product %s: %v\n", p.Symbol, err)
			}
		} else {
			_, err := db.Exec(ctx, `
				UPDATE products 
				SET seller_id = $1, name = $2, category = $3, description = $4, price = $5, stock = $6, status = 'Active', updated_at = now()
				WHERE id = $7
			`, assignedSellerID, p.Name, p.Category, p.Description, p.Price, p.Stock, prodID)
			if err != nil {
				fmt.Printf("Error updating product %s: %v\n", p.Symbol, err)
			}
		}

		if prodID != "" {
			productIDMap[p.Symbol] = prodID
			sellerProductCount[assignedSellerID]++
		}
	}

	for sid, count := range sellerProductCount {
		fmt.Printf("Seller ID %d has %d active products in inventory.\n", sid, count)
	}

	// F. Synchronize 20 symbols into symbols table
	fmt.Println("Synchronizing symbols in symbols table...")
	for _, p := range catalog {
		symCode := fmt.Sprintf("%s_USDT", p.Symbol)
		dispName := fmt.Sprintf("%s / USDT", p.Symbol)
		_, err := db.Exec(ctx, `
			INSERT INTO symbols (symbol, display_name, base_asset, quote_asset, tick_size, lot_size, is_active, created_at)
			VALUES ($1, $2, $3, 'USDT', 1, 1, true, now())
			ON CONFLICT (symbol) DO UPDATE SET 
				display_name = EXCLUDED.display_name,
				base_asset = EXCLUDED.base_asset,
				quote_asset = 'USDT',
				is_active = true
		`, symCode, dispName, p.Symbol)
		if err != nil {
			fmt.Printf("Error updating symbol %s: %v\n", symCode, err)
		}
	}
	// Clean any duplicate symbols rows again
	_, _ = db.Exec(ctx, "DELETE FROM symbols a USING symbols b WHERE a.ctid < b.ctid AND a.symbol = b.symbol")
	fmt.Println("All 20 unique symbols registered and active in symbols table.")

	// G. Seed past purchases from various sellers and decrease prices accordingly
	fmt.Println("Seeding past user purchases from sellers and updating product prices...")
	seedOrders := []struct {
		buyerID int64
		symbol  string
		qty     int
	}{
		{19, "CTG", 2},
		{19, "LEDGER-STX", 1},
		{25, "YUBI-5C", 3},
		{25, "RPI5-NODE", 1},
		{1, "STARLINK", 1},
		{2, "APPLE-SHR", 2},
		{3, "NVDA-SHR", 5},
	}

	for _, o := range seedOrders {
		prodID := productIDMap[o.symbol]
		if prodID == "" {
			_ = db.QueryRow(ctx, "SELECT id FROM products WHERE symbol = $1 LIMIT 1", o.symbol).Scan(&prodID)
		}
		if prodID != "" {
			var originalPrice float64
			var sellerID int64
			_ = db.QueryRow(ctx, "SELECT price, seller_id FROM products WHERE id = $1", prodID).Scan(&originalPrice, &sellerID)

			if originalPrice > 0 {
				totalPrice := originalPrice * float64(o.qty)
				_, _ = db.Exec(ctx, `
					INSERT INTO marketplace_orders (buyer_id, seller_id, product_id, quantity, unit_price, total_price, status, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, 'Completed', now() - interval '2 hours')
				`, o.buyerID, sellerID, prodID, o.qty, originalPrice, totalPrice)

				// Reduce product price by 2% to reflect purchase activity
				reducedPrice := math.Round((originalPrice*0.98)*100) / 100
				_, _ = db.Exec(ctx, "UPDATE products SET price = $1, updated_at = now() WHERE id = $2", reducedPrice, prodID)
				fmt.Printf("Purchased %s: price decreased from $%.2f to $%.2f\n", o.symbol, originalPrice, reducedPrice)
			}
		}
	}

	fmt.Println("=== System Reset and Seeding Completed Successfully! ===")
}
