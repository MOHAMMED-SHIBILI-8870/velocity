package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := "postgres://postgres:1234@localhost:5432/velocity?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	fmt.Println("==================================================")
	fmt.Println("  CLEANING FAKE / DEMO DATA FROM VELOCITY DATABASE")
	fmt.Println("==================================================")

	// 1. Delete fake seed orders (User 19/1/25 dummy seed orders)
	res, err := db.Exec(`
		DELETE FROM orders 
		WHERE id IN (3556001001, 3556001002, 3556001003, 3556001004, 3556001005, 3556001006)
	`)
	if err != nil {
		log.Printf("Error deleting fake seed orders: %v\n", err)
	} else {
		rows, _ := res.RowsAffected()
		fmt.Printf("✓ Deleted %d fake seed orders from 'orders'\n", rows)
	}

	// 2. Delete dummy test positions
	res, err = db.Exec(`DELETE FROM positions WHERE user_id IN (1, 2, 3)`)
	if err != nil {
		log.Printf("Error deleting fake positions: %v\n", err)
	} else {
		rows, _ := res.RowsAffected()
		fmt.Printf("✓ Deleted %d fake positions from 'positions'\n", rows)
	}

	// 3. Delete dummy test accounts (test1, test2, marketmaker, sophia, user3)
	res, err = db.Exec(`DELETE FROM wallets WHERE user_id IN (1, 2, 3, 25, 27)`)
	if err != nil {
		log.Printf("Error deleting fake test wallets: %v\n", err)
	} else {
		rows, _ := res.RowsAffected()
		fmt.Printf("✓ Deleted %d fake test wallets from 'wallets'\n", rows)
	}

	res, err = db.Exec(`DELETE FROM refresh_tokens WHERE user_id IN (1, 2, 3, 25, 27)`)
	if err == nil {
		rows, _ := res.RowsAffected()
		fmt.Printf("✓ Deleted %d refresh tokens for test accounts\n", rows)
	}

	res, err = db.Exec(`DELETE FROM users WHERE id IN (1, 2, 3, 25, 27)`)
	if err != nil {
		log.Printf("Error deleting fake users: %v\n", err)
	} else {
		rows, _ := res.RowsAffected()
		fmt.Printf("✓ Deleted %d fake users from 'users'\n", rows)
	}

	// 4. Clean pre-seeded demo marketplace products and related orders
	_, err = db.Exec(`TRUNCATE marketplace_orders, user_watchlist, products CASCADE`)
	if err != nil {
		log.Printf("Error truncating marketplace demo products: %v\n", err)
	} else {
		fmt.Println("✓ Cleaned demo catalog products, watchlist, and sample marketplace orders")
	}

	// 5. Audit remaining real state
	fmt.Println("\n=== REMAINING ACTIVE USERS ===")
	uRows, err := db.Query("SELECT id, email, full_name, role FROM users ORDER BY id")
	if err == nil {
		for uRows.Next() {
			var id int64
			var email, name, role string
			uRows.Scan(&id, &email, &name, &role)
			fmt.Printf("User %-3d | %-28s | %-20s | %s\n", id, email, name, role)
		}
		uRows.Close()
	}

	fmt.Println("\n=== REMAINING REAL ORDERS ===")
	oRows, err := db.Query("SELECT id, user_id, symbol, side, order_type, price, quantity, status FROM orders ORDER BY id")
	if err == nil {
		count := 0
		for oRows.Next() {
			count++
			var id, uid, price, qty int64
			var sym, side, oType, status string
			oRows.Scan(&id, &uid, &sym, &side, &oType, &price, &qty, &status)
			fmt.Printf("Order %d | User %d | %s %s %s | Price: %d | Qty: %d | Status: %s\n", id, uid, sym, side, oType, price, qty, status)
		}
		if count == 0 {
			fmt.Println("None (All clean!)")
		}
		oRows.Close()
	}

	fmt.Println("\n=== REMAINING ACTIVE BALANCES ===")
	wRows, err := db.Query(`
		SELECT u.id, u.email, w.asset, w.available, w.locked 
		FROM wallets w 
		JOIN users u ON w.user_id = u.id 
		WHERE w.available > 0 OR w.locked > 0
		ORDER BY u.id, w.asset
	`)
	if err == nil {
		for wRows.Next() {
			var uid, avail, locked int64
			var email, asset string
			wRows.Scan(&uid, &email, &asset, &avail, &locked)
			fmt.Printf("User %d (%s) | %s: Available %d, Locked %d\n", uid, email, asset, avail, locked)
		}
		wRows.Close()
	}

	fmt.Println("\n==================================================")
	fmt.Println("  CLEANUP COMPLETE: ALL FAKE DATA REMOVED")
	fmt.Println("==================================================")
}
