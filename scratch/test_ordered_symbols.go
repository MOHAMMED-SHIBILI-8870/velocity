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
		log.Fatal(err)
	}
	defer db.Close()

	query := `
		SELECT 
			s.symbol, s.display_name, s.base_asset,
			COALESCE(p.stock, 0) AS stock,
			t.latest_activity
		FROM symbols s
		LEFT JOIN LATERAL (
			SELECT price, stock, status, updated_at 
			FROM products 
			WHERE symbol = s.base_asset 
			ORDER BY updated_at DESC LIMIT 1
		) p ON true
		LEFT JOIN LATERAL (
			SELECT GREATEST(
				(SELECT MAX(executed_at) FROM trades WHERE symbol = s.symbol),
				(SELECT MAX(created_at) FROM orders WHERE symbol = s.symbol AND side = 'SELL')
			) AS latest_activity
		) t ON true
		WHERE s.is_active = true
		  AND s.symbol LIKE '%\_%'
		ORDER BY 
			CASE 
				WHEN COALESCE(p.stock, 0) > 0 THEN 1 
				WHEN t.latest_activity IS NOT NULL THEN 2 
				ELSE 3 
			END ASC,
			COALESCE(p.stock, 0) DESC,
			t.latest_activity DESC NULLS LAST,
			s.created_at ASC
	`
	rows, err := db.Query(query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("=== ORDERED SYMBOLS ===")
	rank := 1
	for rows.Next() {
		var sym, dName, base string
		var stock int64
		var latest sql.NullString
		rows.Scan(&sym, &dName, &base, &stock, &latest)
		latestStr := "none"
		if latest.Valid {
			latestStr = latest.String
		}
		fmt.Printf("#%02d | %-16s | stock: %4d | activity: %s | %s\n", rank, sym, stock, latestStr, dName)
		rank++
	}
}
