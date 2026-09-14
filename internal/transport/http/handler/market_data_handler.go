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
	Symbol      string  `json:"symbol"`
	DisplayName string  `json:"display_name"`
	BaseAsset   string  `json:"base_asset"`
	QuoteAsset  string  `json:"quote_asset"`
	TickSize    int64   `json:"tick_size"`
	LotSize     int64   `json:"lot_size"`
	Price       float64 `json:"price"`
	IsActive    bool    `json:"is_active"`
}

func (h *MarketDataHandler) Symbols(c *fiber.Ctx) error {
	if h.db != nil {
		rows, err := h.db.Query(c.Context(), `
			SELECT DISTINCT ON (s.symbol)
				s.symbol, s.display_name, s.base_asset, s.quote_asset, 
				s.tick_size, s.lot_size, s.is_active,
				COALESCE(p.price, 0) AS catalog_price
			FROM symbols s
			LEFT JOIN LATERAL (
				SELECT price FROM products WHERE symbol = s.base_asset ORDER BY created_at DESC LIMIT 1
			) p ON true
			WHERE s.is_active = true
			ORDER BY s.symbol, s.created_at ASC
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

	return c.JSON(candleData)
}
