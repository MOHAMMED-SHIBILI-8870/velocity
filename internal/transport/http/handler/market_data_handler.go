package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"velocity/internal/analytics/candles"
	"velocity/internal/domain/depth"
	"velocity/internal/service/marketservice"
	dtoresponse "velocity/internal/transport/http/dto/response"
	"velocity/internal/transport/http/middleware"
	"velocity/pkg/response"
)

type MarketDataHandler struct {
	marketService *marketservice.Service
	db            *pgxpool.Pool
}

func NewMarketDataHandler(
	marketService *marketservice.Service,
	db *pgxpool.Pool,
) *MarketDataHandler {
	return &MarketDataHandler{
		marketService: marketService,
		db:            db,
	}
}

func (h *MarketDataHandler) resolveSymbol(ctx context.Context, input string) string {
	if input == "" || h.db == nil {
		return input
	}
	var canonical string
	err := h.db.QueryRow(ctx, `
		SELECT symbol FROM symbols 
		WHERE symbol = $1 
		   OR UPPER(REPLACE(symbol, '_', '')) = UPPER(REPLACE($1, '_', ''))
		ORDER BY 
		   CASE WHEN symbol = $1 THEN 0 
		        WHEN symbol LIKE '%\_%' THEN 1 
		        ELSE 2 
		   END
		LIMIT 1
	`, input).Scan(&canonical)
	if err == nil && canonical != "" {
		return canonical
	}
	return input
}

// GetOrderBook godoc
//
//	@Summary		Get order book
//	@Description	Returns the current order book for a symbol
//	@Tags			Market Data
//	@Produce		json
//	@Param			symbol	path		string	true	"Trading Symbol"
//	@Param			limit	query		int		false	"Depth limit"	default(20)
//	@Success		200		{object}	response.OrderBookResponse
//	@Failure		400		{object}	response.ErrorResponse
//	@Failure		404		{object}	response.ErrorResponse
//	@Router			/api/orderbook/{symbol} [get]
func (h *MarketDataHandler) GetOrderBook(c *fiber.Ctx) error {

	rawSymbol := c.Params("symbol")
	if rawSymbol == "" {
		return fiber.NewError(
			fiber.StatusBadRequest,
			"symbol is required",
		)
	}

	symbol := h.resolveSymbol(c.Context(), rawSymbol)
	limit := c.QueryInt("limit", 20)

	orderBook, err := h.marketService.GetOrderBook(
		context.Background(),
		symbol,
		limit,
	)
	if err != nil {
		// If symbol is registered in DB, return an empty depth book rather than failing
		if h.db != nil {
			var exists bool
			_ = h.db.QueryRow(c.Context(), "SELECT EXISTS(SELECT 1 FROM symbols WHERE symbol = $1)", symbol).Scan(&exists)
			if exists {
				return c.Status(fiber.StatusOK).JSON(
					dtoresponse.OrderBookResponse{
						Symbol: symbol,
						Bids:   []depth.Level{},
						Asks:   []depth.Level{},
					},
				)
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "symbol not found",
		})
	}

	return c.Status(fiber.StatusOK).JSON(
		dtoresponse.OrderBookResponse{
			Symbol: orderBook.Symbol,
			Bids:   orderBook.Bids,
			Asks:   orderBook.Asks,
		},
	)
}

func (h *MarketDataHandler) GetTicker(c *fiber.Ctx) error {

	rawSymbol := c.Params("symbol")
	symbol := h.resolveSymbol(c.Context(), rawSymbol)

	ticker, err := h.marketService.GetTicker(context.Background(), symbol)
	if err != nil {
		// Fallback to product catalog price from database
		if h.db != nil {
			var catPrice float64
			errDb := h.db.QueryRow(c.Context(), `
				SELECT COALESCE(p.price, 0)
				FROM symbols s
				LEFT JOIN products p ON p.symbol = s.base_asset
				WHERE s.symbol = $1
				LIMIT 1
			`, symbol).Scan(&catPrice)
			if errDb == nil && catPrice > 0 {
				return c.JSON(fiber.Map{
					"symbol":     symbol,
					"last_price": int64(catPrice),
					"best_bid":   int64(catPrice),
					"best_ask":   int64(catPrice),
					"price":      catPrice,
				})
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// If no trades executed yet, fallback to product catalog price
	if ticker.LastPrice == 0 && h.db != nil {
		var catPrice float64
		_ = h.db.QueryRow(c.Context(), `
			SELECT COALESCE(p.price, 0)
			FROM products p
			INNER JOIN symbols s ON s.base_asset = p.symbol
			WHERE s.symbol = $1
			LIMIT 1
		`, symbol).Scan(&catPrice)
		if catPrice > 0 {
			ticker.LastPrice = int64(catPrice)
		}
	}

	return c.JSON(ticker)
}

func (h *MarketDataHandler) GetRecentTrades(c *fiber.Ctx) error {

	rawSymbol := c.Params("symbol")
	symbol := h.resolveSymbol(c.Context(), rawSymbol)

	trades, err := h.marketService.GetRecentTrades(
		c.Context(),
		symbol,
	)

	if err != nil {
		return c.JSON([]interface{}{})
	}

	return c.JSON(trades)
}

type MarketSymbolDTO struct {
	Symbol          string  `json:"symbol"`
	DisplayName     string  `json:"display_name"`
	BaseAsset       string  `json:"base_asset"`
	QuoteAsset      string  `json:"quote_asset"`
	TickSize        int64   `json:"tick_size"`
	LotSize         int64   `json:"lot_size"`
	Price           float64 `json:"price"`
	Stock           int64   `json:"stock"`
	Status          string  `json:"status"`
	LatestTradeTime int64   `json:"latest_trade_time"`
	IsActive        bool    `json:"is_active"`
}

func (h *MarketDataHandler) Symbols(c *fiber.Ctx) error {
	if h.db != nil {
		rows, err := h.db.Query(c.Context(), `
			SELECT 
				s.symbol, s.display_name, s.base_asset, s.quote_asset, 
				s.tick_size, s.lot_size, s.is_active,
				COALESCE(p.price, 0) AS catalog_price,
				COALESCE(p.stock, 0) AS stock,
				COALESCE(p.status, 'Active') AS product_status,
				COALESCE(EXTRACT(EPOCH FROM t.latest_activity)::BIGINT, 0) AS latest_trade_time
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
		`)
		if err == nil {
			defer rows.Close()
			var list []MarketSymbolDTO
			for rows.Next() {
				var item MarketSymbolDTO
				var catPrice float64
				if err := rows.Scan(
					&item.Symbol, &item.DisplayName, &item.BaseAsset, &item.QuoteAsset,
					&item.TickSize, &item.LotSize, &item.IsActive, &catPrice,
					&item.Stock, &item.Status, &item.LatestTradeTime,
				); err == nil {
					ticker, tErr := h.marketService.GetTicker(c.Context(), item.Symbol)
					if tErr == nil && ticker != nil && ticker.LastPrice > 0 {
						item.Price = float64(ticker.LastPrice)
					} else {
						item.Price = catPrice
					}
					list = append(list, item)
				}
			}
			if len(list) > 0 {
				return c.JSON(list)
			}
		}
	}

	symbols, err := h.marketService.GetSymbols(
		c.Context(),
	)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{
				"error": err.Error(),
			})
	}

	return c.JSON(symbols)
}

func (h *MarketDataHandler) GetUserTrades(c *fiber.Ctx) error {

	userID := middleware.GetUserID(c)

	if userID == 0 {
		return response.Error(
			c,
			fiber.StatusUnauthorized,
			"invalid user",
			"user not found in authentication context",
		)
	}

	trades, err := h.marketService.GetUserTrades(
		c.Context(),
		userID,
	)

	if err != nil {
		return response.Error(
			c,
			fiber.StatusInternalServerError,
			"failed to retrieve trades",
			err.Error(),
		)
	}

	return response.Success(
		c,
		fiber.StatusOK,
		"trades retrieved successfully",
		trades,
	)
}

func (h *MarketDataHandler) GetMarketStats(c *fiber.Ctx) error {

	rawSymbol := c.Params("symbol")
	symbol := h.resolveSymbol(c.Context(), rawSymbol)

	stats, err := h.marketService.GetMarketStats(symbol)
	if err != nil {
		if h.db != nil {
			var catPrice float64
			errDb := h.db.QueryRow(c.Context(), `
				SELECT COALESCE(p.price, 0)
				FROM symbols s
				LEFT JOIN products p ON p.symbol = s.base_asset
				WHERE s.symbol = $1
				LIMIT 1
			`, symbol).Scan(&catPrice)
			if errDb == nil && catPrice > 0 {
				return c.JSON(fiber.Map{
					"symbol":       symbol,
					"last_price":   int64(catPrice),
					"high_price":   int64(catPrice),
					"low_price":    int64(catPrice),
					"open_price":   int64(catPrice),
					"change24h":    0.0,
					"quote_volume": 0.0,
				})
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if stats.LastPrice == 0 && h.db != nil {
		var catPrice float64
		_ = h.db.QueryRow(c.Context(), `
			SELECT COALESCE(p.price, 0)
			FROM products p
			INNER JOIN symbols s ON s.base_asset = p.symbol
			WHERE s.symbol = $1
			LIMIT 1
		`, symbol).Scan(&catPrice)
		if catPrice > 0 {
			stats.LastPrice = int64(catPrice)
			stats.HighPrice = int64(catPrice)
			stats.LowPrice = int64(catPrice)
			stats.OpenPrice = int64(catPrice)
		}
	}

	return c.JSON(stats)
}

func computeISTBucket(t time.Time, interval string) (time.Time, time.Time) {
	loc := time.FixedZone("IST", 5*3600+30*60)
	tIST := t.In(loc)

	var bucketStart time.Time
	var dur time.Duration
	switch interval {
	case "1m":
		dur = time.Minute
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), tIST.Minute(), 0, 0, loc)
	case "5m":
		dur = 5 * time.Minute
		minute := (tIST.Minute() / 5) * 5
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), minute, 0, 0, loc)
	case "15m":
		dur = 15 * time.Minute
		minute := (tIST.Minute() / 15) * 15
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), minute, 0, 0, loc)
	case "1h":
		dur = time.Hour
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), tIST.Hour(), 0, 0, 0, loc)
	case "4h":
		dur = 4 * time.Hour
		hour := (tIST.Hour() / 4) * 4
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), hour, 0, 0, 0, loc)
	case "1d":
		dur = 24 * time.Hour
		bucketStart = time.Date(tIST.Year(), tIST.Month(), tIST.Day(), 0, 0, 0, 0, loc)
	default:
		dur = 15 * time.Minute
		bucketStart = tIST.Truncate(dur)
	}
	return bucketStart, bucketStart.Add(dur)
}

func (h *MarketDataHandler) GetCandles(
	c *fiber.Ctx,
) error {

	const (
		defaultLimit = 500
		maxLimit     = 1000
	)

	rawSymbol := c.Params("symbol")
	if rawSymbol == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error": "symbol is required",
			},
		)
	}

	symbol := h.resolveSymbol(c.Context(), rawSymbol)

	intervalStr := c.Query("interval")

	if intervalStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error": "interval is required",
			},
		)
	}

	interval := candles.Interval(intervalStr)

	if !interval.IsValid() {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error":               "invalid interval",
				"supported_intervals": candles.SupportedIntervals,
			},
		)
	}

	limit := c.QueryInt("limit", defaultLimit)

	if limit <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error": "limit must be greater than 0",
			},
		)
	}

	if limit > maxLimit {
		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error": "limit must not exceed 1000",
			},
		)
	}

	var startTime *time.Time
	var endTime *time.Time

	startTimeStr := c.Query("startTime")

	if startTimeStr != "" {
		parsed, err := time.Parse(
			time.RFC3339,
			startTimeStr,
		)

		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				fiber.Map{
					"error": "invalid startTime; expected RFC3339 timestamp",
				},
			)
		}

		startTime = &parsed
	}

	endTimeStr := c.Query("endTime")

	if endTimeStr != "" {
		parsed, err := time.Parse(
			time.RFC3339,
			endTimeStr,
		)

		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(
				fiber.Map{
					"error": "invalid endTime; expected RFC3339 timestamp",
				},
			)
		}

		endTime = &parsed
	}

	if startTime != nil &&
		endTime != nil &&
		startTime.After(*endTime) {

		return c.Status(fiber.StatusBadRequest).JSON(
			fiber.Map{
				"error": "startTime must not be after endTime",
			},
		)
	}

	candleData, err := h.marketService.GetCandles(
		symbol,
		interval,
		limit,
		startTime,
		endTime,
	)

	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(
			fiber.Map{
				"error": err.Error(),
			},
		)
	}

	if h.db != nil {
		rows, qErr := h.db.Query(c.Context(), `
			SELECT symbol, interval, open_time, close_time, open, high, low, close, volume, quote_volume, trade_count
			FROM (
				SELECT symbol, interval, open_time, close_time, open, high, low, close, volume, quote_volume, trade_count
				FROM candles
				WHERE symbol = $1 AND interval = $2
				ORDER BY open_time DESC
				LIMIT $3
			) sub
			ORDER BY open_time ASC
		`, symbol, string(interval), limit)
		if qErr == nil {
			defer rows.Close()
			var dbCandles []*candles.Candle
			for rows.Next() {
				var cd candles.Candle
				var invStr string
				var tc int64
				if scanErr := rows.Scan(
					&cd.Symbol, &invStr, &cd.OpenTime, &cd.CloseTime,
					&cd.Open, &cd.High, &cd.Low, &cd.Close,
					&cd.Volume, &cd.QuoteVolume, &tc,
				); scanErr == nil {
					cd.Interval = candles.Interval(invStr)
					cd.TradeCount = uint64(tc)
					dbCandles = append(dbCandles, &cd)
				}
			}

			// Query trades table for any trades since latest candle's OpenTime
			var sinceTime time.Time
			if len(dbCandles) > 0 {
				sinceTime = dbCandles[len(dbCandles)-1].OpenTime
			}
			tRows, tErr := h.db.Query(c.Context(), `
				SELECT price, quantity, executed_at 
				FROM trades 
				WHERE symbol = $1 AND executed_at >= $2 
				ORDER BY executed_at ASC
			`, symbol, sinceTime)
			if tErr == nil {
				defer tRows.Close()
				for tRows.Next() {
					var tPrice, tQty float64
					var tTime time.Time
					if err := tRows.Scan(&tPrice, &tQty, &tTime); err == nil {
						bStart, bEnd := computeISTBucket(tTime, string(interval))
						pInt := int64(tPrice)
						qInt := int64(tQty)
						if len(dbCandles) > 0 && dbCandles[len(dbCandles)-1].OpenTime.Equal(bStart) {
							cur := dbCandles[len(dbCandles)-1]
							cur.Close = pInt
							if pInt > cur.High {
								cur.High = pInt
							}
							if pInt < cur.Low {
								cur.Low = pInt
							}
						} else {
							newCd := &candles.Candle{
								Symbol:      symbol,
								Interval:    interval,
								OpenTime:    bStart,
								CloseTime:   bEnd,
								Open:        pInt,
								High:        pInt,
								Low:         pInt,
								Close:       pInt,
								Volume:      qInt,
								QuoteVolume: int64(tPrice * tQty),
								TradeCount:  1,
							}
							dbCandles = append(dbCandles, newCd)
							go func(cd candles.Candle) {
								_, _ = h.db.Exec(context.Background(), `
									INSERT INTO candles (symbol, interval, open_time, close_time, open, high, low, close, volume, quote_volume, trade_count)
									VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
									ON CONFLICT (symbol, interval, open_time) DO UPDATE SET
										close_time = EXCLUDED.close_time,
										high = GREATEST(candles.high, EXCLUDED.high),
										low = LEAST(candles.low, EXCLUDED.low),
										close = EXCLUDED.close,
										volume = EXCLUDED.volume,
										quote_volume = EXCLUDED.quote_volume,
										trade_count = EXCLUDED.trade_count
								`, cd.Symbol, string(cd.Interval), cd.OpenTime, cd.CloseTime, cd.Open, cd.High, cd.Low, cd.Close, cd.Volume, cd.QuoteVolume, cd.TradeCount)
							}(*newCd)
						}
					}
				}
			}

			if len(dbCandles) > 0 {
				candleData = dbCandles
			}
		}
	}

	return c.JSON(candleData)
}

func (h *MarketDataHandler) GetSymbol(c *fiber.Ctx) error {

	symbol := c.Params("symbol")

	if symbol == "" {
		return response.Error(
			c,
			fiber.StatusBadRequest,
			"invalid symbol",
			"symbol is required",
		)
	}

	symbolData, err := h.marketService.GetSymbol(
		c.Context(),
		symbol,
	)

	if err != nil {
		return response.Error(
			c,
			fiber.StatusNotFound,
			"symbol not found",
			err.Error(),
		)
	}

	return response.Success(
		c,
		fiber.StatusOK,
		"symbol retrieved successfully",
		symbolData,
	)
}
