package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://postgres:1234@localhost:5432/velocity?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	fmt.Println("==================================================")
	fmt.Println("  SYNCING SELLER INVENTORY TO EXCHANGE ORDERBOOK  ")
	fmt.Println("==================================================")

	// 1. Credit Seller 21's wallet with 12 S21-PRO tokens in the database
	_, err = db.Exec(`
		INSERT INTO wallets (user_id, asset, available, locked)
		VALUES (21, 'S21-PRO', 12, 0)
		ON CONFLICT (user_id, asset) DO UPDATE
		SET available = EXCLUDED.available
	`)
	if err != nil {
		log.Fatalf("Failed to credit seller wallet: %v", err)
	}
	fmt.Println("✓ Credited Seller 21 wallet with 12 S21-PRO tokens")

	// 2. Ensure symbol S21-PRO_USDT exists and is active
	_, err = db.Exec(`
		INSERT INTO symbols (symbol, display_name, base_asset, quote_asset, tick_size, lot_size, is_active, created_at)
		VALUES ('S21-PRO_USDT', 'Antminer S21 Pro Miner / USDT', 'S21-PRO', 'USDT', 1, 1, true, now())
		ON CONFLICT (symbol) DO UPDATE SET is_active = true
	`)
	if err == nil {
		fmt.Println("✓ Verified symbol S21-PRO_USDT is active")
	}

	// 3. Log in as Seller 21 (seller@example.com / password) to get access token
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "seller@example.com",
		"password": "password",
	})
	resp, err := http.Post("http://localhost:8081/api/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		log.Fatalf("Failed to call login endpoint: %v", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var loginRes struct {
		Data struct {
			AccessToken string `json:"access_token"`
			User        struct {
				ID int64 `json:"id"`
			} `json:"user"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(respBytes, &loginRes)

	token := loginRes.Data.AccessToken
	if token == "" {
		token = loginRes.AccessToken
	}
	if token == "" {
		log.Fatalf("Failed to obtain seller token: %s", string(respBytes))
	}
	fmt.Printf("✓ Obtained seller JWT access token for user ID %d\n", loginRes.Data.User.ID)

	// 4. Submit SELL LIMIT order for 12 S21-PRO @ $3,800 to Velocity Engine
	orderPayload, _ := json.Marshal(map[string]interface{}{
		"symbol":        "S21-PRO_USDT",
		"side":          "SELL",
		"type":          "LIMIT",
		"time_in_force": "GTC",
		"price":         3800,
		"quantity":      12,
	})

	req, err := http.NewRequest("POST", "http://localhost:8080/api/orders", bytes.NewBuffer(orderPayload))
	if err != nil {
		log.Fatalf("Failed to create order request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	orderResp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Failed to send order request to Velocity: %v", err)
	}
	defer orderResp.Body.Close()

	orderBody, _ := io.ReadAll(orderResp.Body)
	fmt.Printf("Order submission response (status %d): %s\n", orderResp.StatusCode, string(orderBody))

	// 5. Inspect matched orders in DB
	fmt.Println("\n=== S21-PRO ORDERS STATUS AFTER MATCHING ===")
	rows, err := db.Query(`
		SELECT id, user_id, side, price, quantity, remaining, filled, status 
		FROM orders 
		WHERE symbol = 'S21-PRO_USDT'
		ORDER BY id
	`)
	if err == nil {
		for rows.Next() {
			var id, uid, price, qty, rem, filled int64
			var side, status string
			rows.Scan(&id, &uid, &side, &price, &qty, &rem, &filled, &status)
			fmt.Printf("Order %-20d | User %-3d | %-4s | Price: %-5d | Qty: %-2d | Rem: %-2d | Filled: %-2d | Status: %s\n",
				id, uid, side, price, qty, rem, filled, status)
		}
		rows.Close()
	}

	fmt.Println("\n==================================================")
	fmt.Println("  SYNC COMPLETE                                   ")
	fmt.Println("==================================================")
}
