package marketplaceservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"velocity/internal/service/orderservice"
	"velocity/internal/service/walletservice"
	"velocity/pkg/constants"
)

type Product struct {
	ID          string    `json:"id"`
	SellerID    int64     `json:"seller_id"`
	Name        string    `json:"name"`
	Symbol      string    `json:"symbol"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	LockedStock int       `json:"locked"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	IsMonitored bool      `json:"is_monitored"`
}

type SellerStats struct {
	TotalRevenue         float64 `json:"totalRevenue"`
	TotalProductsSold    int     `json:"totalProductsSold"`
	ActiveListings       int     `json:"activeListings"`
	LockedInventoryValue float64 `json:"lockedInventoryValue"`
}

type SellerActivity struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Action   string    `json:"action"`
	Product  string    `json:"product"`
	Price    float64   `json:"price"`
	Quantity int       `json:"quantity"`
}

type OrderResult struct {
	OrderID    string    `json:"order_id"`
	ProductID  string    `json:"product_id"`
	Name       string    `json:"name"`
	Quantity   int       `json:"quantity"`
	UnitPrice  float64   `json:"unit_price"`
	TotalPrice float64   `json:"total_price"`
	CreatedAt  time.Time `json:"created_at"`
}

type CreateProductParams struct {
	Name        string  `json:"name"`
	Symbol      string  `json:"symbol"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Stock       int     `json:"stock"`
}

type Service struct {
	db            *pgxpool.Pool
	walletService *walletservice.Service
	orderService  *orderservice.Service
}

func New(db *pgxpool.Pool, walletService *walletservice.Service, orderService *orderservice.Service) *Service {
	return &Service{
		db:            db,
		walletService: walletService,
		orderService:  orderService,
	}
}

// ListProducts returns active marketplace products, with an optional user watchlist indicator.
func (s *Service) ListProducts(ctx context.Context, userID int64, category, search string) ([]Product, error) {
	query := `
		SELECT 
			p.id, p.seller_id, p.name, p.symbol, p.category, p.description, 
			p.price, p.stock, p.locked_stock, p.status, p.created_at, p.updated_at,
			EXISTS(SELECT 1 FROM user_watchlist w WHERE w.product_id = p.id AND w.user_id = $1) as is_monitored
		FROM products p
		WHERE p.status = 'Active'
	`
	args := []interface{}{userID}
	argIdx := 2

	if category != "" && category != "All" {
		query += fmt.Sprintf(" AND p.category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}

	if search != "" {
		query += fmt.Sprintf(" AND (p.name ILIKE $%d OR p.symbol ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	query += " ORDER BY p.created_at DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
			&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
			&p.IsMonitored,
		); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, p)
	}

	if products == nil {
		products = []Product{}
	}
	return products, nil
}

// GetUserWatchlist returns all products the user has added to their watchlist.
func (s *Service) GetUserWatchlist(ctx context.Context, userID int64) ([]Product, error) {
	query := `
		SELECT 
			p.id, p.seller_id, p.name, p.symbol, p.category, p.description, 
			p.price, p.stock, p.locked_stock, p.status, p.created_at, p.updated_at,
			true as is_monitored
		FROM products p
		INNER JOIN user_watchlist w ON p.id = w.product_id
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC
	`
	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query watchlist: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
			&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
			&p.IsMonitored,
		); err != nil {
			return nil, fmt.Errorf("scan watchlist product: %w", err)
		}
		products = append(products, p)
	}

	if products == nil {
		products = []Product{}
	}
	return products, nil
}

// ToggleWatchlist toggles a product into or out of the user's watchlist.
func (s *Service) ToggleWatchlist(ctx context.Context, userID int64, productID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM user_watchlist WHERE user_id = $1 AND product_id = $2)
	`, userID, productID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check watchlist: %w", err)
	}

	if exists {
		_, err := s.db.Exec(ctx, "DELETE FROM user_watchlist WHERE user_id = $1 AND product_id = $2", userID, productID)
		if err != nil {
			return false, fmt.Errorf("remove from watchlist: %w", err)
		}
		return false, nil
	}

	_, err = s.db.Exec(ctx, "INSERT INTO user_watchlist (user_id, product_id) VALUES ($1, $2)", userID, productID)
	if err != nil {
		return false, fmt.Errorf("add to watchlist: %w", err)
	}
	return true, nil
}

// BuyProduct executes an atomic purchase: validates stock, settles USDT balances, deducts stock, and logs order.
func (s *Service) BuyProduct(ctx context.Context, buyerID int64, productID string, quantity int) (*OrderResult, error) {
	if quantity <= 0 {
		return nil, errors.New("quantity must be greater than zero")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		sellerID    int64
		name        string
		price       float64
		stock       int
		status      string
	)

	err = tx.QueryRow(ctx, `
		SELECT seller_id, name, price, stock, status
		FROM products
		WHERE id = $1
		FOR UPDATE
	`, productID).Scan(&sellerID, &name, &price, &stock, &status)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("product not found")
		}
		return nil, fmt.Errorf("fetch product: %w", err)
	}

	if status != "Active" || stock < quantity {
		return nil, fmt.Errorf("insufficient stock: available %d, requested %d", stock, quantity)
	}

	if buyerID == sellerID {
		return nil, errors.New("seller cannot purchase their own product")
	}

	totalPrice := price * float64(quantity)
	costInt := int64(math.Round(totalPrice))
	if costInt <= 0 {
		costInt = 1
	}

	// Withdraw funds from buyer's USDT wallet
	if err := s.walletService.Withdraw(ctx, buyerID, "USDT", costInt); err != nil {
		return nil, fmt.Errorf("payment withdrawal failed: %w", err)
	}

	// Deposit funds to seller's USDT wallet
	if err := s.walletService.Deposit(ctx, sellerID, "USDT", costInt); err != nil {
		return nil, fmt.Errorf("failed to credit seller wallet: %w", err)
	}

	// Update product inventory and dynamically decrease price upon purchase
	newStock := stock - quantity
	newStatus := status
	if newStock == 0 {
		newStatus = "Sold Out"
	}

	newPrice := math.Round(price * 0.98 * 100) / 100
	if newPrice < 1.0 {
		newPrice = 1.0
	}

	_, err = tx.Exec(ctx, `
		UPDATE products 
		SET stock = $1, status = $2, price = $3, updated_at = now()
		WHERE id = $4
	`, newStock, newStatus, newPrice, productID)
	if err != nil {
		return nil, fmt.Errorf("update product stock and price: %w", err)
	}

	// Log marketplace order
	var orderID string
	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO marketplace_orders (
			buyer_id, seller_id, product_id, quantity, unit_price, total_price, status
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'Completed'
		) RETURNING id, created_at
	`, buyerID, sellerID, productID, quantity, price, totalPrice).Scan(&orderID, &createdAt)

	if err != nil {
		return nil, fmt.Errorf("record order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &OrderResult{
		OrderID:    orderID,
		ProductID:  productID,
		Name:       name,
		Quantity:   quantity,
		UnitPrice:  price,
		TotalPrice: totalPrice,
		CreatedAt:  createdAt,
	}, nil
}

// GetSellerProducts retrieves all products listed by the seller.
func (s *Service) GetSellerProducts(ctx context.Context, sellerID int64) ([]Product, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 
			id, seller_id, name, symbol, category, description, 
			price, stock, locked_stock, status, created_at, updated_at
		FROM products
		WHERE seller_id = $1
		ORDER BY created_at DESC
	`, sellerID)
	if err != nil {
		return nil, fmt.Errorf("query seller products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(
			&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
			&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan seller product: %w", err)
		}
		products = append(products, p)
	}

	if products == nil {
		products = []Product{}
	}
	return products, nil
}

// CreateProduct adds a new product for a seller.
func (s *Service) CreateProduct(ctx context.Context, sellerID int64, params CreateProductParams) (*Product, error) {
	category := params.Category
	if category == "" {
		category = "General"
	}

	var p Product
	err := s.db.QueryRow(ctx, `
		INSERT INTO products (
			seller_id, name, symbol, category, description, price, stock, locked_stock, status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, 0, 'Active'
		) RETURNING id, seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at
	`, sellerID, params.Name, params.Symbol, category, params.Description, params.Price, params.Stock).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}

	// Synchronize canonical market symbol into symbols table
	marketSymbolUnderscore := fmt.Sprintf("%s_USDT", p.Symbol)
	displayName := fmt.Sprintf("%s / USDT", p.Name)
	_, _ = s.db.Exec(ctx, `
		INSERT INTO symbols (symbol, display_name, base_asset, quote_asset, tick_size, lot_size, is_active, created_at)
		VALUES ($1, $2, $3, 'USDT', 1, 1, true, now())
		ON CONFLICT (symbol) DO UPDATE
		SET display_name = EXCLUDED.display_name, base_asset = EXCLUDED.base_asset, is_active = true
	`, marketSymbolUnderscore, displayName, p.Symbol)

	// Auto-provision exchange liquidity if stock is provided
	if p.Stock > 0 && s.orderService != nil {
		_ = s.walletService.Deposit(ctx, sellerID, p.Symbol, int64(p.Stock))

		priceInt := int64(math.Round(p.Price))
		if priceInt <= 0 {
			priceInt = 1
		}
		_, err = s.orderService.Submit(ctx, orderservice.SubmitOrderRequest{
			UserID:      sellerID,
			Symbol:      marketSymbolUnderscore,
			Side:        constants.OrderSideSell,
			Type:        constants.OrderTypeLimit,
			TimeInForce: constants.TimeInForceGTC,
			Price:       priceInt,
			Quantity:    int64(p.Stock),
		})
		if err != nil {
			log.Printf("[marketplaceservice] Auto sell order placement failed: %v", err)
		}
	}

	return &p, nil
}

// UpdateProduct updates product details or stock.
func (s *Service) UpdateProduct(ctx context.Context, sellerID int64, productID string, params CreateProductParams) (*Product, error) {
	var p Product
	status := "Active"
	if params.Stock == 0 {
		status = "Sold Out"
	}

	err := s.db.QueryRow(ctx, `
		UPDATE products
		SET name = $1, symbol = $2, price = $3, stock = $4, status = $5, updated_at = now()
		WHERE id = $6 AND seller_id = $7
		RETURNING id, seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at
	`, params.Name, params.Symbol, params.Price, params.Stock, status, productID, sellerID).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}

	// Synchronize market symbol
	marketSymbol := fmt.Sprintf("%s_USDT", p.Symbol)
	displayName := fmt.Sprintf("%s / USDT", p.Name)
	_, _ = s.db.Exec(ctx, `
		INSERT INTO symbols (symbol, display_name, base_asset, quote_asset, tick_size, lot_size, is_active, created_at)
		VALUES ($1, $2, $3, 'USDT', 1, 1, $4, now())
		ON CONFLICT (symbol) DO UPDATE
		SET display_name = EXCLUDED.display_name, base_asset = EXCLUDED.base_asset, is_active = $4
	`, marketSymbol, displayName, p.Symbol, status == "Active")

	return &p, nil
}

// DeleteProduct deletes a product owned by the seller.
func (s *Service) DeleteProduct(ctx context.Context, sellerID int64, productID string) error {
	res, err := s.db.Exec(ctx, `DELETE FROM products WHERE id = $1 AND seller_id = $2`, productID, sellerID)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	if res.RowsAffected() == 0 {
		return errors.New("product not found or unauthorized")
	}
	return nil
}

// GetSellerStats aggregates live sales, product count, and revenue metrics.
func (s *Service) GetSellerStats(ctx context.Context, sellerID int64) (*SellerStats, error) {
	var (
		totalRevenue      sql.NullFloat64
		totalProductsSold sql.NullInt64
		activeListings    int
		lockedValue       sql.NullFloat64
	)

	// Sales metrics
	err := s.db.QueryRow(ctx, `
		SELECT SUM(total_price), SUM(quantity)
		FROM marketplace_orders
		WHERE seller_id = $1
	`, sellerID).Scan(&totalRevenue, &totalProductsSold)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query sales stats: %w", err)
	}

	// Active listings
	err = s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM products
		WHERE seller_id = $1 AND status = 'Active'
	`, sellerID).Scan(&activeListings)
	if err != nil {
		return nil, fmt.Errorf("query active listings: %w", err)
	}

	// Locked value
	err = s.db.QueryRow(ctx, `
		SELECT SUM(price * locked_stock)
		FROM products
		WHERE seller_id = $1
	`, sellerID).Scan(&lockedValue)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query locked value: %w", err)
	}

	return &SellerStats{
		TotalRevenue:         totalRevenue.Float64,
		TotalProductsSold:    int(totalProductsSold.Int64),
		ActiveListings:       activeListings,
		LockedInventoryValue: lockedValue.Float64,
	}, nil
}

// GetSellerActivity returns recent sales made by the seller.
func (s *Service) GetSellerActivity(ctx context.Context, sellerID int64) ([]SellerActivity, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.created_at, p.name, o.total_price, o.quantity
		FROM marketplace_orders o
		INNER JOIN products p ON o.product_id = p.id
		WHERE o.seller_id = $1
		ORDER BY o.created_at DESC
		LIMIT 20
	`, sellerID)
	if err != nil {
		return nil, fmt.Errorf("query seller activity: %w", err)
	}
	defer rows.Close()

	var activities []SellerActivity
	for rows.Next() {
		var a SellerActivity
		if err := rows.Scan(&a.ID, &a.Time, &a.Product, &a.Price, &a.Quantity); err != nil {
			return nil, fmt.Errorf("scan seller activity: %w", err)
		}
		a.Action = fmt.Sprintf("Sold (%d unit%s)", a.Quantity, func() string {
			if a.Quantity > 1 {
				return "s"
			}
			return ""
		}())
		activities = append(activities, a)
	}

	if activities == nil {
		activities = []SellerActivity{}
	}
	return activities, nil
}

type BuyerOrder struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id"`
	ProductName string    `json:"product_name"`
	Symbol      string    `json:"symbol"`
	Category    string    `json:"category"`
	Quantity    int       `json:"quantity"`
	UnitPrice   float64   `json:"unit_price"`
	TotalPrice  float64   `json:"total_price"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetBuyerOrders returns all purchases made by the buyer.
func (s *Service) GetBuyerOrders(ctx context.Context, buyerID int64) ([]BuyerOrder, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 
			o.id, o.product_id, p.name, p.symbol, p.category, 
			o.quantity, o.unit_price, o.total_price, o.status, o.created_at
		FROM marketplace_orders o
		INNER JOIN products p ON o.product_id = p.id
		WHERE o.buyer_id = $1
		ORDER BY o.created_at DESC
	`, buyerID)
	if err != nil {
		return nil, fmt.Errorf("query buyer orders: %w", err)
	}
	defer rows.Close()

	var orders []BuyerOrder
	for rows.Next() {
		var o BuyerOrder
		if err := rows.Scan(
			&o.ID, &o.ProductID, &o.ProductName, &o.Symbol, &o.Category,
			&o.Quantity, &o.UnitPrice, &o.TotalPrice, &o.Status, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan buyer order: %w", err)
		}
		orders = append(orders, o)
	}

	if orders == nil {
		orders = []BuyerOrder{}
	}
	return orders, nil
}

// GetProductByID returns a single product by its UUID
func (s *Service) GetProductByID(ctx context.Context, productID string) (*Product, error) {
	var p Product
	err := s.db.QueryRow(ctx, `
		SELECT 
			id, seller_id, name, symbol, category, description, 
			price, stock, locked_stock, status, created_at, updated_at
		FROM products
		WHERE id = $1
	`, productID).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("product not found")
		}
		return nil, fmt.Errorf("fetch product: %w", err)
	}
	return &p, nil
}

// ToggleProductStatus toggles product status between 'Active' and 'Inactive'
func (s *Service) ToggleProductStatus(ctx context.Context, sellerID int64, productID string, newStatus string) (*Product, error) {
	var p Product
	if newStatus == "" {
		var curStatus string
		if err := s.db.QueryRow(ctx, "SELECT status FROM products WHERE id = $1 AND seller_id = $2", productID, sellerID).Scan(&curStatus); err != nil {
			return nil, errors.New("product not found or unauthorized")
		}
		if curStatus == "Active" {
			newStatus = "Inactive"
		} else {
			newStatus = "Active"
		}
	}

	err := s.db.QueryRow(ctx, `
		UPDATE products
		SET status = $1, updated_at = now()
		WHERE id = $2 AND seller_id = $3
		RETURNING id, seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at
	`, newStatus, productID, sellerID).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("toggle status: %w", err)
	}

	// Sync market symbol active status
	marketSymbol := fmt.Sprintf("%sUSDT", p.Symbol)
	_, _ = s.db.Exec(ctx, "UPDATE symbols SET is_active = $1 WHERE symbol = $2", newStatus == "Active", marketSymbol)

	return &p, nil
}

type InventoryItem struct {
	ProductID         string `json:"product_id"`
	Name              string `json:"name"`
	Symbol            string `json:"symbol"`
	Category          string `json:"category"`
	TotalStock        int    `json:"total_stock"`
	AvailableStock    int    `json:"available_stock"`
	LockedStock       int    `json:"locked_stock"`
	SoldQuantity      int    `json:"sold_quantity"`
	LowStockThreshold int    `json:"low_stock_threshold"`
	IsLowStock        bool   `json:"is_low_stock"`
	Status            string `json:"status"`
}

// GetSellerInventory returns the inventory items for a seller
func (s *Service) GetSellerInventory(ctx context.Context, sellerID int64) ([]InventoryItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 
			p.id, p.name, p.symbol, p.category, 
			p.stock, p.locked_stock, p.status,
			COALESCE(SUM(o.quantity), 0) AS sold_qty
		FROM products p
		LEFT JOIN marketplace_orders o ON o.product_id = p.id
		WHERE p.seller_id = $1
		GROUP BY p.id, p.name, p.symbol, p.category, p.stock, p.locked_stock, p.status, p.created_at
		ORDER BY p.created_at DESC
	`, sellerID)
	if err != nil {
		return nil, fmt.Errorf("query inventory: %w", err)
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		var soldQty int64
		if err := rows.Scan(
			&item.ProductID, &item.Name, &item.Symbol, &item.Category,
			&item.AvailableStock, &item.LockedStock, &item.Status, &soldQty,
		); err != nil {
			return nil, fmt.Errorf("scan inventory: %w", err)
		}
		item.TotalStock = item.AvailableStock + item.LockedStock
		item.SoldQuantity = int(soldQty)
		item.LowStockThreshold = 5
		item.IsLowStock = item.AvailableStock <= item.LowStockThreshold && item.AvailableStock > 0
		items = append(items, item)
	}
	if items == nil {
		items = []InventoryItem{}
	}
	return items, nil
}

// AddStock increases product stock
func (s *Service) AddStock(ctx context.Context, sellerID int64, productID string, quantity int) (*Product, error) {
	if quantity <= 0 {
		return nil, errors.New("quantity must be greater than zero")
	}

	var p Product
	err := s.db.QueryRow(ctx, `
		UPDATE products
		SET stock = stock + $1,
		    status = CASE WHEN status = 'Sold Out' THEN 'Active' ELSE status END,
		    updated_at = now()
		WHERE id = $2 AND seller_id = $3
		RETURNING id, seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at
	`, quantity, productID, sellerID).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add stock: %w", err)
	}

	if quantity > 0 && s.orderService != nil {
		_ = s.walletService.Deposit(ctx, sellerID, p.Symbol, int64(quantity))
		priceInt := int64(math.Round(p.Price))
		if priceInt <= 0 {
			priceInt = 1
		}
		marketSymbolUnderscore := fmt.Sprintf("%s_USDT", p.Symbol)
		_, _ = s.orderService.Submit(ctx, orderservice.SubmitOrderRequest{
			UserID:      sellerID,
			Symbol:      marketSymbolUnderscore,
			Side:        constants.OrderSideSell,
			Type:        constants.OrderTypeLimit,
			TimeInForce: constants.TimeInForceGTC,
			Price:       priceInt,
			Quantity:    int64(quantity),
		})
	}

	return &p, nil
}

// AdjustStock overrides product stock count
func (s *Service) AdjustStock(ctx context.Context, sellerID int64, productID string, newStock int) (*Product, error) {
	if newStock < 0 {
		return nil, errors.New("stock cannot be negative")
	}

	var p Product
	status := "Active"
	if newStock == 0 {
		status = "Sold Out"
	}

	err := s.db.QueryRow(ctx, `
		UPDATE products
		SET stock = $1, status = $2, updated_at = now()
		WHERE id = $3 AND seller_id = $4
		RETURNING id, seller_id, name, symbol, category, description, price, stock, locked_stock, status, created_at, updated_at
	`, newStock, status, productID, sellerID).Scan(
		&p.ID, &p.SellerID, &p.Name, &p.Symbol, &p.Category, &p.Description,
		&p.Price, &p.Stock, &p.LockedStock, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("adjust stock: %w", err)
	}
	return &p, nil
}

type SellerOrder struct {
	ID            string    `json:"id"`
	ProductID     string    `json:"product_id"`
	ProductName   string    `json:"product_name"`
	ProductSymbol string    `json:"product_symbol"`
	Quantity      int       `json:"quantity"`
	UnitPrice     float64   `json:"unit_price"`
	TotalPrice    float64   `json:"total_price"`
	BuyerName     string    `json:"buyer_name"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   time.Time `json:"completed_at"`
}

// GetSellerOrders returns sales orders for this seller
func (s *Service) GetSellerOrders(ctx context.Context, sellerID int64, statusFilter string, search string) ([]SellerOrder, error) {
	query := `
		SELECT 
			o.id, o.product_id, p.name, p.symbol,
			o.quantity, o.unit_price, o.total_price,
			COALESCE(u.email, 'Buyer #' || o.buyer_id) AS buyer_name,
			o.status, o.created_at
		FROM marketplace_orders o
		INNER JOIN products p ON o.product_id = p.id
		LEFT JOIN users u ON o.buyer_id = u.id
		WHERE o.seller_id = $1
	`
	args := []any{sellerID}

	if statusFilter != "" && statusFilter != "All" {
		args = append(args, statusFilter)
		query += fmt.Sprintf(" AND o.status = $%d", len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(" AND (p.name ILIKE $%d OR p.symbol ILIKE $%d)", len(args), len(args))
	}

	query += " ORDER BY o.created_at DESC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query seller orders: %w", err)
	}
	defer rows.Close()

	var orders []SellerOrder
	for rows.Next() {
		var o SellerOrder
		if err := rows.Scan(
			&o.ID, &o.ProductID, &o.ProductName, &o.ProductSymbol,
			&o.Quantity, &o.UnitPrice, &o.TotalPrice,
			&o.BuyerName, &o.Status, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan seller order: %w", err)
		}
		o.CompletedAt = o.CreatedAt
		orders = append(orders, o)
	}
	if orders == nil {
		orders = []SellerOrder{}
	}
	return orders, nil
}

// GetSellerOrderDetails returns single order details
func (s *Service) GetSellerOrderDetails(ctx context.Context, sellerID int64, orderID string) (*SellerOrder, error) {
	var o SellerOrder
	err := s.db.QueryRow(ctx, `
		SELECT 
			o.id, o.product_id, p.name, p.symbol,
			o.quantity, o.unit_price, o.total_price,
			COALESCE(u.email, 'Buyer #' || o.buyer_id) AS buyer_name,
			o.status, o.created_at
		FROM marketplace_orders o
		INNER JOIN products p ON o.product_id = p.id
		LEFT JOIN users u ON o.buyer_id = u.id
		WHERE o.id = $1 AND o.seller_id = $2
	`, orderID, sellerID).Scan(
		&o.ID, &o.ProductID, &o.ProductName, &o.ProductSymbol,
		&o.Quantity, &o.UnitPrice, &o.TotalPrice,
		&o.BuyerName, &o.Status, &o.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("order not found")
		}
		return nil, fmt.Errorf("fetch order: %w", err)
	}
	o.CompletedAt = o.CreatedAt
	return &o, nil
}

// GetSellerWallet returns seller wallet balances and metrics
func (s *Service) GetSellerWallet(ctx context.Context, sellerID int64) (map[string]any, error) {
	var available, locked int64
	_ = s.db.QueryRow(ctx, `
		SELECT available, locked FROM wallets WHERE user_id = $1 AND asset = 'USDT'
	`, sellerID).Scan(&available, &locked)

	stats, _ := s.GetSellerStats(ctx, sellerID)
	totalEarnings := 0.0
	totalSales := 0
	if stats != nil {
		totalEarnings = stats.TotalRevenue
		totalSales = stats.TotalProductsSold
	}

	return map[string]any{
		"availableBalance": float64(available),
		"pendingBalance":   float64(locked),
		"totalEarnings":    totalEarnings,
		"totalSales":       totalSales,
		"platformFees":     0,
		"netEarnings":      totalEarnings,
	}, nil
}

// GetSellerWalletTransactions returns sales transactions
func (s *Service) GetSellerWalletTransactions(ctx context.Context, sellerID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id, o.created_at, p.name, o.total_price
		FROM marketplace_orders o
		INNER JOIN products p ON o.product_id = p.id
		WHERE o.seller_id = $1
		ORDER BY o.created_at DESC
		LIMIT 50
	`, sellerID)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id string
		var t time.Time
		var prod string
		var amount float64
		if err := rows.Scan(&id, &t, &prod, &amount); err == nil {
			list = append(list, map[string]any{
				"id":           id,
				"type":         "Sale",
				"description":  "Sale of " + prod,
				"amount":       amount,
				"status":       "Completed",
				"reference_id": id,
				"created_at":   t,
			})
		}
	}
	if list == nil {
		list = []map[string]any{}
	}
	return list, nil
}

// GetSellerWithdrawals returns withdrawal records
func (s *Service) GetSellerWithdrawals(ctx context.Context, sellerID int64) ([]map[string]any, error) {
	return []map[string]any{}, nil
}

// RequestSellerWithdrawal processes a withdrawal
func (s *Service) RequestSellerWithdrawal(ctx context.Context, sellerID int64, amount float64, method string, accountInfo string) (map[string]any, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}
	costInt := int64(math.Round(amount))
	if err := s.walletService.Withdraw(ctx, sellerID, "USDT", costInt); err != nil {
		return nil, fmt.Errorf("withdrawal failed: %w", err)
	}

	return map[string]any{
		"id":           fmt.Sprintf("WD-%d", time.Now().Unix()),
		"amount":       amount,
		"method":       method,
		"account_info": accountInfo,
		"status":       "Pending",
		"created_at":   time.Now(),
	}, nil
}

// GetSellerAlerts returns low stock alerts
func (s *Service) GetSellerAlerts(ctx context.Context, sellerID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, symbol, stock
		FROM products
		WHERE seller_id = $1 AND stock <= 5
		ORDER BY stock ASC
	`, sellerID)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var alerts []map[string]any
	for rows.Next() {
		var id, name, symbol string
		var stock int
		if err := rows.Scan(&id, &name, &symbol, &stock); err == nil {
			alerts = append(alerts, map[string]any{
				"product_id": id,
				"name":       name,
				"symbol":     symbol,
				"stock":      stock,
				"threshold":  5,
				"message":    fmt.Sprintf("Low stock — only %d units remaining.", stock),
			})
		}
	}
	if alerts == nil {
		alerts = []map[string]any{}
	}
	return alerts, nil
}

