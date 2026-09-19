package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Trade struct {
	ID         int64
	Symbol     string
	Price      float64
	Quantity   float64
	ExecutedAt time.Time
}

type IntervalConfig struct {
	Name     string
	Duration time.Duration
}

var intervals = []IntervalConfig{
	{"1m", 1 * time.Minute},
	{"5m", 5 * time.Minute},
	{"15m", 15 * time.Minute},
	{"1h", 1 * time.Hour},
	{"4h", 4 * time.Hour},
	{"1d", 24 * time.Hour},
}

type CandleAgg struct {
	Symbol      string
	Interval    string
	OpenTime    time.Time
	CloseTime   time.Time
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	QuoteVolume float64
	TradeCount  int64
}

func main() {
	dsn := "postgres://postgres:1234@localhost:5432/velocity?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("failed to connect to DB: %v", err)
	}
	defer db.Close()

	// 1. Delete alias non-underscore symbols from symbols table
	_, _ = db.Exec("DELETE FROM symbols WHERE symbol = 'CTGUSDT' OR symbol = 'RPC-12USDT'")
	fmt.Println("Deleted alias non-underscore symbols CTGUSDT and RPC-12USDT from symbols table.")

	// 2. Fetch all trades ordered by executed_at ASC
	rows, err := db.Query("SELECT id, symbol, price, quantity, executed_at FROM trades ORDER BY executed_at ASC")
	if err != nil {
		log.Fatalf("failed to query trades: %v", err)
	}
	defer rows.Close()

	var trades []Trade
	for rows.Next() {
		var t Trade
		if err := rows.Scan(&t.ID, &t.Symbol, &t.Price, &t.Quantity, &t.ExecutedAt); err != nil {
			log.Fatalf("scan trade err: %v", err)
		}
		trades = append(trades, t)
	}
	fmt.Printf("Loaded %d total trades from database.\n", len(trades))

	// 3. Clear existing candles table to do a clean re-aggregation
	_, err = db.Exec("DELETE FROM candles")
	if err != nil {
		log.Fatalf("failed to clear candles table: %v", err)
	}

	loc := time.FixedZone("IST", 5*3600+30*60)

	// 4. Aggregate trades for each interval
	for _, inv := range intervals {
		aggMap := make(map[string]*CandleAgg)
		var orderedKeys []string

		for _, t := range trades {
			tIST := t.ExecutedAt.In(loc)
			var bucketStart time.Time
			switch inv.Name {
			case "1m":
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), tIST.Minute(), 0, 0, loc)
			case "5m":
				minute := (tIST.Minute() / 5) * 5
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), minute, 0, 0, loc)
			case "15m":
				minute := (tIST.Minute() / 15) * 15
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), minute, 0, 0, loc)
			case "1h":
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), 0, 0, 0, loc)
			case "4h":
				hour := (tIST.Hour() / 4) * 4
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), hour, 0, 0, 0, loc)
			case "1d":
				bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), 0, 0, 0, 0, loc)
			default:
				bucketStart = tIST.Truncate(inv.Duration)
			}
			bucketEnd := bucketStart.Add(inv.Duration)
			key := fmt.Sprintf("%s_%s_%d", t.Symbol, inv.Name, bucketStart.Unix())

			if agg, exists := aggMap[key]; exists {
				if t.Price > agg.High {
					agg.High = t.Price
				}
				if t.Price < agg.Low {
					agg.Low = t.Price
				}
				agg.Close = t.Price
				agg.Volume += t.Quantity
				agg.QuoteVolume += t.Price * t.Quantity
				agg.TradeCount++
			} else {
				orderedKeys = append(orderedKeys, key)
				aggMap[key] = &CandleAgg{
					Symbol:      t.Symbol,
					Interval:    inv.Name,
					OpenTime:    bucketStart,
					CloseTime:   bucketEnd,
					Open:        t.Price,
					High:        t.Price,
					Low:         t.Price,
					Close:       t.Price,
					Volume:      t.Quantity,
					QuoteVolume: t.Price * t.Quantity,
					TradeCount:  1,
				}
			}
		}

		// Insert candles for this interval
		for _, key := range orderedKeys {
			c := aggMap[key]
			_, err := db.Exec(`
				INSERT INTO candles (symbol, interval, open_time, close_time, open, high, low, close, volume, quote_volume, trade_count)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
				ON CONFLICT (symbol, interval, open_time) DO UPDATE SET
					close_time = EXCLUDED.close_time,
					open = EXCLUDED.open,
					high = EXCLUDED.high,
					low = EXCLUDED.low,
					close = EXCLUDED.close,
					volume = EXCLUDED.volume,
					quote_volume = EXCLUDED.quote_volume,
					trade_count = EXCLUDED.trade_count
			`, c.Symbol, c.Interval, c.OpenTime, c.CloseTime, c.Open, c.High, c.Low, c.Close, c.Volume, c.QuoteVolume, c.TradeCount)
			if err != nil {
				log.Printf("insert candle error (%s %s): %v\n", c.Symbol, c.Interval, err)
			}
		}
		fmt.Printf("Interval %-3s: Aggregated and inserted %d candles.\n", inv.Name, len(orderedKeys))
	}

	fmt.Println("All historical candles successfully re-aggregated and saved to PostgreSQL.")
}
