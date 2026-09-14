package router

import (
	// identityclient "velocity/internal/transport/grpc/client/identity"
	"velocity/internal/transport/http/handler"
	"velocity/internal/transport/http/middleware"
	// "velocity/internal/transport/http/middleware"

	"github.com/gofiber/fiber/v2"
)

func Register(
	app *fiber.App,
	orderHandler *handler.OrderHandler,
	marketHandler *handler.MarketDataHandler,
	walletHandler *handler.WalletHandler,
	positionHandler *handler.PositionHandler,
	healthHandler *handler.HealthHandler,
	adminHandler *handler.AdminHandler,
	marketplaceHandler *handler.MarketplaceHandler,
	paymentHandler *handler.PaymentHandler,
	auth fiber.Handler,
	optionalAuth fiber.Handler,
	requireAdmin fiber.Handler,
	rateLimit *middleware.RateLimitMiddleware,
) {
	// Health checks live at the top level, not under /api, since
	// orchestrators (Docker, k8s) and load balancers conventionally
	// probe /health directly, and it must never require auth.
	RegisterHealthRoutes(app, healthHandler)

	api := app.Group("/api")

	// Public Market Routes
	RegisterMarketRoutes(api, marketHandler)

	// Protected Trading Routes
	api.Get("/market/trades/user", auth, marketHandler.GetUserTrades)
	RegisterOrderRoutes(api, orderHandler, rateLimit, auth)
	RegisterPaymentRoutes(api, paymentHandler, auth)
	RegisterWalletRoutes(api, walletHandler, auth)
	RegisterPositionRoutes(api, positionHandler, auth)

	// Marketplace routes
	marketplace := api.Group("/marketplace")
	marketplace.Get("/products", optionalAuth, marketplaceHandler.ListProducts)
	marketplace.Get("/orders", auth, marketplaceHandler.GetBuyerOrders)
	marketplace.Get("/watchlist", auth, marketplaceHandler.GetUserWatchlist)
	marketplace.Post("/watchlist/:id", auth, marketplaceHandler.ToggleWatchlist)
	marketplace.Post("/buy", auth, marketplaceHandler.BuyProduct)

	// Seller routes
	seller := api.Group("/seller", auth)
	seller.Get("/products", marketplaceHandler.GetSellerProducts)
	seller.Get("/products/:id", marketplaceHandler.GetProductByID)
	seller.Post("/products", marketplaceHandler.CreateSellerProduct)
	seller.Put("/products/:id", marketplaceHandler.UpdateSellerProduct)
	seller.Patch("/products/:id/status", marketplaceHandler.ToggleProductStatus)
	seller.Put("/products/:id/status", marketplaceHandler.ToggleProductStatus)
	seller.Delete("/products/:id", marketplaceHandler.DeleteSellerProduct)

	// Inventory
	seller.Get("/inventory", marketplaceHandler.GetSellerInventory)
	seller.Post("/inventory/:id/add", marketplaceHandler.AddStock)
	seller.Post("/inventory/:id/adjust", marketplaceHandler.AdjustStock)
	seller.Get("/inventory/:id/history", marketplaceHandler.GetInventoryHistory)

	// Orders
	seller.Get("/orders", marketplaceHandler.GetSellerOrders)
	seller.Get("/orders/:id", marketplaceHandler.GetSellerOrderDetails)

	// Wallet & Finances
	seller.Get("/wallet", marketplaceHandler.GetSellerWallet)
	seller.Get("/wallet/transactions", marketplaceHandler.GetSellerWalletTransactions)
	seller.Get("/withdrawals", marketplaceHandler.GetSellerWithdrawals)
	seller.Post("/withdrawals", marketplaceHandler.RequestSellerWithdrawal)

	// Alerts & Stats
	seller.Get("/stats", marketplaceHandler.GetSellerStats)
	seller.Get("/activity", marketplaceHandler.GetSellerActivity)
	seller.Get("/alerts", marketplaceHandler.GetSellerAlerts)

	// Admin - auth first, then role check, in that order, so an
	// unauthenticated caller gets 401 rather than 403.
	RegisterAdminRoutes(api, adminHandler, auth, requireAdmin)
}
