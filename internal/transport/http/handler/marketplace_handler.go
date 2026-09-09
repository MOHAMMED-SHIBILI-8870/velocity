package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"velocity/internal/service/marketplaceservice"
	"velocity/internal/transport/http/middleware"
	"velocity/pkg/response"
)

type MarketplaceHandler struct {
	service *marketplaceservice.Service
}

func NewMarketplaceHandler(service *marketplaceservice.Service) *MarketplaceHandler {
	return &MarketplaceHandler{
		service: service,
	}
}

// resolveUserID extracts the authenticated user ID from context, falling back to X-User-Id or user_id param.
func resolveUserID(c *fiber.Ctx) int64 {
	userID := middleware.GetUserID(c)
	if userID > 0 {
		return userID
	}
	if uidStr := c.Get("X-User-Id"); uidStr != "" {
		if id, err := strconv.ParseInt(uidStr, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	if uidStr := c.Query("user_id"); uidStr != "" {
		if id, err := strconv.ParseInt(uidStr, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// ListProducts: GET /api/marketplace/products?category=...&search=...
func (h *MarketplaceHandler) ListProducts(c *fiber.Ctx) error {
	userID := resolveUserID(c)
	category := c.Query("category")
	search := c.Query("search")

	products, err := h.service.ListProducts(c.Context(), userID, category, search)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch marketplace products", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "products retrieved", products)
}

// GetUserWatchlist: GET /api/marketplace/watchlist
func (h *MarketplaceHandler) GetUserWatchlist(c *fiber.Ctx) error {
	userID := resolveUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	watchlist, err := h.service.GetUserWatchlist(c.Context(), userID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch watchlist", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "watchlist retrieved", watchlist)
}

// ToggleWatchlist: POST /api/marketplace/watchlist/:id
func (h *MarketplaceHandler) ToggleWatchlist(c *fiber.Ctx) error {
	userID := resolveUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	if productID == "" {
		return response.BadRequest(c, "missing product id")
	}

	isMonitored, err := h.service.ToggleWatchlist(c.Context(), userID, productID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to toggle watchlist", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "watchlist updated", fiber.Map{
		"product_id":   productID,
		"is_monitored": isMonitored,
	})
}

type BuyProductRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// BuyProduct: POST /api/marketplace/buy
func (h *MarketplaceHandler) BuyProduct(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	var req BuyProductRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if req.ProductID == "" {
		return response.BadRequest(c, "product_id is required")
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}

	order, err := h.service.BuyProduct(c.Context(), userID, req.ProductID, req.Quantity)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "purchase failed", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "purchase completed successfully", order)
}

// GetBuyerOrders: GET /api/marketplace/orders
func (h *MarketplaceHandler) GetBuyerOrders(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	orders, err := h.service.GetBuyerOrders(c.Context(), userID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch orders", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "orders retrieved", orders)
}

// -------------------------------------------------------------
// Seller Endpoints
// -------------------------------------------------------------

// GetSellerProducts: GET /api/seller/products
func (h *MarketplaceHandler) GetSellerProducts(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	products, err := h.service.GetSellerProducts(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch seller products", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "seller products retrieved", products)
}

// CreateSellerProduct: POST /api/seller/products
func (h *MarketplaceHandler) CreateSellerProduct(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	var params marketplaceservice.CreateProductParams
	if err := c.BodyParser(&params); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if params.Name == "" || params.Symbol == "" || params.Price <= 0 {
		return response.BadRequest(c, "name, symbol, and valid price are required")
	}

	p, err := h.service.CreateProduct(c.Context(), user.UserID, params)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to create product", err.Error())
	}

	return response.Success(c, fiber.StatusCreated, "product created successfully", p)
}

// UpdateSellerProduct: PUT /api/seller/products/:id
func (h *MarketplaceHandler) UpdateSellerProduct(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	if productID == "" {
		return response.BadRequest(c, "product id required")
	}

	var params marketplaceservice.CreateProductParams
	if err := c.BodyParser(&params); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	p, err := h.service.UpdateProduct(c.Context(), user.UserID, productID, params)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to update product", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "product updated successfully", p)
}

// DeleteSellerProduct: DELETE /api/seller/products/:id
func (h *MarketplaceHandler) DeleteSellerProduct(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	if productID == "" {
		return response.BadRequest(c, "product id required")
	}

	if err := h.service.DeleteProduct(c.Context(), user.UserID, productID); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "failed to delete product", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "product deleted successfully", fiber.Map{"id": productID})
}

// GetSellerStats: GET /api/seller/stats
func (h *MarketplaceHandler) GetSellerStats(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	stats, err := h.service.GetSellerStats(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch seller stats", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "seller stats retrieved", stats)
}

// GetSellerActivity: GET /api/seller/activity
func (h *MarketplaceHandler) GetSellerActivity(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	activity, err := h.service.GetSellerActivity(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch seller activity", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "seller activity retrieved", activity)
}

// GetProductByID: GET /api/seller/products/:id
func (h *MarketplaceHandler) GetProductByID(c *fiber.Ctx) error {
	productID := c.Params("id")
	if productID == "" {
		return response.BadRequest(c, "product id required")
	}

	p, err := h.service.GetProductByID(c.Context(), productID)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "product not found", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "product retrieved", p)
}

type ToggleStatusRequest struct {
	Status string `json:"status"`
}

// ToggleProductStatus: PATCH /api/seller/products/:id/status
func (h *MarketplaceHandler) ToggleProductStatus(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	if productID == "" {
		return response.BadRequest(c, "product id required")
	}

	var req ToggleStatusRequest
	_ = c.BodyParser(&req)

	p, err := h.service.ToggleProductStatus(c.Context(), user.UserID, productID, req.Status)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "failed to update product status", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "product status updated", p)
}

// GetSellerInventory: GET /api/seller/inventory
func (h *MarketplaceHandler) GetSellerInventory(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	inventory, err := h.service.GetSellerInventory(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch inventory", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "inventory retrieved", inventory)
}

type StockActionRequest struct {
	Quantity int    `json:"quantity"`
	NewStock int    `json:"new_stock"`
	Reason   string `json:"reason"`
}

// AddStock: POST /api/seller/inventory/:id/add
func (h *MarketplaceHandler) AddStock(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	var req StockActionRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid body")
	}

	p, err := h.service.AddStock(c.Context(), user.UserID, productID, req.Quantity)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "failed to add stock", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "stock added successfully", p)
}

// AdjustStock: POST /api/seller/inventory/:id/adjust
func (h *MarketplaceHandler) AdjustStock(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	productID := c.Params("id")
	var req StockActionRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid body")
	}

	p, err := h.service.AdjustStock(c.Context(), user.UserID, productID, req.NewStock)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "failed to adjust stock", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "stock adjusted successfully", p)
}

// GetInventoryHistory: GET /api/seller/inventory/:id/history
func (h *MarketplaceHandler) GetInventoryHistory(c *fiber.Ctx) error {
	return response.Success(c, fiber.StatusOK, "inventory history retrieved", []fiber.Map{
		{
			"id":             "log-1",
			"change_type":    "Initial Stock",
			"quantity":       0,
			"previous_stock": 0,
			"new_stock":      0,
			"reason":         "Initial product cataloging",
			"created_at":     "2026-09-08T00:00:00Z",
		},
	})
}

// GetSellerOrders: GET /api/seller/orders
func (h *MarketplaceHandler) GetSellerOrders(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	status := c.Query("status")
	search := c.Query("search")

	orders, err := h.service.GetSellerOrders(c.Context(), user.UserID, status, search)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch seller orders", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "seller orders retrieved", orders)
}

// GetSellerOrderDetails: GET /api/seller/orders/:id
func (h *MarketplaceHandler) GetSellerOrderDetails(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	orderID := c.Params("id")
	order, err := h.service.GetSellerOrderDetails(c.Context(), user.UserID, orderID)
	if err != nil {
		return response.Error(c, fiber.StatusNotFound, "order not found", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "order details retrieved", order)
}

// GetSellerWallet: GET /api/seller/wallet
func (h *MarketplaceHandler) GetSellerWallet(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	wallet, err := h.service.GetSellerWallet(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch seller wallet", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "seller wallet retrieved", wallet)
}

// GetSellerWalletTransactions: GET /api/seller/wallet/transactions
func (h *MarketplaceHandler) GetSellerWalletTransactions(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	txs, err := h.service.GetSellerWalletTransactions(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch wallet transactions", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "transactions retrieved", txs)
}

// GetSellerWithdrawals: GET /api/seller/withdrawals
func (h *MarketplaceHandler) GetSellerWithdrawals(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	withdrawals, err := h.service.GetSellerWithdrawals(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch withdrawals", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "withdrawals retrieved", withdrawals)
}

type WithdrawalRequest struct {
	Amount      float64 `json:"amount"`
	Method      string  `json:"method"`
	AccountInfo string  `json:"account_info"`
}

// RequestSellerWithdrawal: POST /api/seller/withdrawals
func (h *MarketplaceHandler) RequestSellerWithdrawal(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	var req WithdrawalRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid body")
	}

	res, err := h.service.RequestSellerWithdrawal(c.Context(), user.UserID, req.Amount, req.Method, req.AccountInfo)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "withdrawal failed", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "withdrawal requested", res)
}

// GetSellerAlerts: GET /api/seller/alerts
func (h *MarketplaceHandler) GetSellerAlerts(c *fiber.Ctx) error {
	user, err := middleware.GetAuthenticatedUser(c)
	if err != nil || user == nil {
		return response.Unauthorized(c, "authentication required")
	}

	alerts, err := h.service.GetSellerAlerts(c.Context(), user.UserID)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to fetch alerts", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "alerts retrieved", alerts)
}

