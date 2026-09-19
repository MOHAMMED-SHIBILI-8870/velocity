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

	rows, err := db.Query(ctx, "SELECT column_name FROM information_schema.columns WHERE table_name='users'")
	if err == nil {
		fmt.Println("Columns in users:")
		for rows.Next() {
			var col string
			rows.Scan(&col)
			fmt.Printf("Col: %s\n", col)
		}
		rows.Close()
	}

	uRows, err := db.Query(ctx, "SELECT user_id, asset, available, locked FROM wallets LIMIT 20")
	if err == nil {
		fmt.Println("Rows in wallets:")
		for uRows.Next() {
			var uid int64
			var asset string
			var avail, locked int64
			uRows.Scan(&uid, &asset, &avail, &locked)
			fmt.Printf("Wallet: user=%d | %s | avail=%d | locked=%d\n", uid, asset, avail, locked)
		}
		uRows.Close()
	}

	wRows, err := db.Query(ctx, "SELECT user_id, product_id, created_at FROM user_watchlist")
	if err == nil {
		fmt.Println("Rows in user_watchlist:")
		for wRows.Next() {
			var uid int64
			var pid string
			var t interface{}
			wRows.Scan(&uid, &pid, &t)
			fmt.Printf("Watchlist: user_id=%d | product_id=%s\n", uid, pid)
		}
		wRows.Close()
	}
}
