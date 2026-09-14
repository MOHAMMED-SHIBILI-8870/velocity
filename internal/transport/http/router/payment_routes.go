package router

import (
	"github.com/gofiber/fiber/v2"
	"velocity/internal/transport/http/handler"
)

func RegisterPaymentRoutes(
	api fiber.Router,
	paymentHandler *handler.PaymentHandler,
	auth fiber.Handler,
) {
	payments := api.Group("/payments")
	payments.Post("/razorpay/webhook", paymentHandler.Webhook)
	payments.Post("/razorpay/create-order", auth, paymentHandler.CreateOrder)
	payments.Post("/razorpay/verify", auth, paymentHandler.VerifyPayment)
	payments.Get("/transactions", auth, paymentHandler.ListTransactions)

	wallets := api.Group("/wallets", auth)
	wallets.Post("/withdraw-cash", paymentHandler.RequestWithdrawal)
	wallets.Get("/transactions", paymentHandler.ListTransactions)
}
