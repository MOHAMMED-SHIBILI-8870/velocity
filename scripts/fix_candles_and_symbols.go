package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	dsn := "host=localhost port=5432 dbname=velocity user=postgres password=1234 sslmode=disable TimeZone=UTC"
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}
	defer pool.Close()

	fmt.Println("=== 1. CREATING CANDLES TABLE IF NOT EXISTS ===")
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS candles (
			symbol TEXT NOT NULL REFERENCES symbols(symbol),
			interval TEXT NOT NULL,
			open_time  TIMESTAMPTZ NOT NULL,
			close_time TIMESTAMPTZ NOT NULL,
			open  BIGINT NOT NULL,
			high  BIGINT NOT NULL,
			low   BIGINT NOT NULL,
			close BIGINT NOT NULL,
			volume       BIGINT NOT NULL,
			quote_volume BIGINT NOT NULL,
			trade_count  BIGINT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (symbol, interval, open_time)
		);
		CREATE INDEX IF NOT EXISTS idx_candles_symbol_interval_time
		ON candles(symbol, interval, open_time DESC);
	`)
	if err != nil {
		log.Fatalf("failed to create candles table: %v", err)
	}
	fmt.Println("✓ Candles table ready.")

	fmt.Println("\n=== 2. REMOVING / DEACTIVATING BOGUS LEGACY SYMBOLS WITHOUT UNDERSCORE ===")
	// Find symbols where an underscored version exists (e.g. CTGUSDT vs CTG_USDT)
	res, err := pool.Exec(ctx, `
		DELETE FROM symbols 
		WHERE symbol NOT LIKE '%\_%' 
		  AND symbol NOT IN ('BTCUSDT', 'ETHUSDT', 'SOLUSDT', 'BNBUSDT')
		  AND EXISTS (
			SELECT 1 FROM symbols s2 
			WHERE s2.symbol LIKE '%\_%' 
			  AND REPLACE(s2.symbol, '_', '') = symbols.symbol
		  );
	`)
	if err != nil {
		fmt.Printf("Warning deleting legacy symbols: %v\n", err)
	} else {
		fmt.Printf("✓ Deleted %d legacy duplicate symbols without underscore.\n", res.RowsAffected())
	}

	fmt.Println("\n=== 3. BACKFILLING CANDLES FROM TRADES ===")
	type TradeItem struct {
		Symbol     string
		Price      int64
		Quantity   int64
		ExecutedAt time.Time
	}

	rows, err := pool.Query(ctx, `
		SELECT symbol, price, quantity, executed_at 
		FROM trades 
		ORDER BY executed_at ASC, id ASC
	`)
	if err != nil {
		log.Fatalf("failed to query trades: %v", err)
	}
	defer rows.Close()

	var allTrades []TradeItem
	for rows.Next() {
		var t TradeItem
		if err := rows.Scan(&t.Symbol, &t.Price, &t.Quantity, &t.ExecutedAt); err == nil {
			allTrades = append(allTrades, t)
		}
	}
	fmt.Printf("Found %d trades to process.\n", len(allTrades))

	intervals := []struct {
		name string
		dur  time.Duration
	}{
		{"1m", 1 * time.Minute},
		{"5m", 5 * time.Minute},
		{"15m", 15 * time.Minute},
		{"1h", 1 * time.Hour},
		{"4h", 4 * time.Hour},
		{"1d", 24 * time.Hour},
	}

	type CandleBucket struct {
		symbol      string
		interval    string
		openTime    time.Time
		closeTime   time.Time
		open        int64
		high        int64
		low         int64
		close       int64
		volume      int64
		quoteVolume int64
		tradeCount  int64
	}

	buckets := make(map[string]*CandleBucket)

	for _, t := range allTrades {
		for _, inv := range intervals {
			start := t.ExecutedAt.UTC().Truncate(inv.dur)
			end := start.Add(inv.dur)
			key := fmt.Sprintf("%s:%s:%d", t.Symbol, inv.name, start.Unix())

			b, exists := buckets[key]
			if !exists {
				b = &CandleBucket{
					symbol:      t.Symbol,
					interval:    inv.name,
					openTime:    start,
					closeTime:   end,
					open:        t.Price,
					high:        t.Price,
					low:         t.Price,
					close:       t.Price,
					volume:      t.Quantity,
					quoteVolume: t.Price * t.Quantity,
					tradeCount:  1,
				}
				buckets[key] = b
			} else {
				if t.Price > b.high {
					b.high = t.Price
				}
				if t.Price < b.low {
					b.low = t.Price
				}
				b.close = t.Price
				b.volume += t.Quantity
				b.quoteVolume += t.Price * t.Quantity
				b.tradeCount++
			}
		}
	}

	fmt.Printf("Generated %d candle records. Inserting/upserting into 'candles' table...\n", len(buckets))
	upsertCount := 0
	for _, b := range buckets {
		_, err := pool.Exec(ctx, `
			INSERT INTO candles (
				symbol, interval, open_time, close_time,
				open, high, low, close,
				volume, quote_volume, trade_count, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
			ON CONFLICT (symbol, interval, open_time) DO UPDATE SET
				high = EXCLUDED.high,
				low = EXCLUDED.low,
				close = EXCLUDED.close,
				volume = EXCLUDED.volume,
				quote_volume = EXCLUDED.quote_volume,
				trade_count = EXCLUDED.trade_count,
				updated_at = now()
		`, b.symbol, b.interval, b.openTime, b.closeTime, b.open, b.high, b.low, b.close, b.volume, b.quoteVolume, b.tradeCount)
		if err != nil {
			fmt.Printf("Error inserting candle for %s %s: %v\n", b.symbol, b.interval, err)
		} else {
			upsertCount++
		}
	}
	fmt.Printf("✓ Successfully upserted %d candles into Postgres!\n", upsertCount)
}
