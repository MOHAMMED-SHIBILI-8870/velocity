package router

import (
	"github.com/gofiber/fiber/v2"

	"velocity/internal/transport/http/handler"
)

func RegisterWalletRoutes(
	api fiber.Router,
	walletHandler *handler.WalletHandler,
	auth fiber.Handler,
) {
	wallet := api.Group("/wallets", auth)

	wallet.Get("/", walletHandler.List)

	wallet.Get("/:asset", walletHandler.GetByAsset)

	wallet.Post("/deposit", walletHandler.Deposit)

	wallet.Post("/withdraw", walletHandler.Withdraw)
}
