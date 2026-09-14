package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"velocity/internal/service/paymentservice"
	"velocity/internal/transport/http/middleware"
	"velocity/pkg/response"
)

type PaymentHandler struct {
	service *paymentservice.Service
}

func NewPaymentHandler(service *paymentservice.Service) *PaymentHandler {
	return &PaymentHandler{
		service: service,
	}
}

type CreateOrderRequest struct {
	Amount float64 `json:"amount"` // in INR
}

// CreateOrder: POST /api/payments/razorpay/create-order
func (h *PaymentHandler) CreateOrder(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	var req CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if req.Amount <= 0 {
		return response.BadRequest(c, "amount must be greater than 0")
	}

	res, err := h.service.CreateDepositOrder(c.Context(), userID, req.Amount)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to create payment order", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "payment order created", res)
}

// VerifyPayment: POST /api/payments/razorpay/verify
func (h *PaymentHandler) VerifyPayment(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	var req paymentservice.VerifyPaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	res, err := h.service.VerifyDepositPayment(c.Context(), userID, req)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "payment verification failed", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "payment verified successfully", res)
}

// RequestWithdrawal: POST /api/wallets/withdraw-cash
func (h *PaymentHandler) RequestWithdrawal(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	var req paymentservice.WithdrawalRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if req.Amount <= 0 {
		return response.BadRequest(c, "amount must be greater than 0")
	}

	res, err := h.service.RequestWithdrawal(c.Context(), userID, req)
	if err != nil {
		return response.Error(c, fiber.StatusBadRequest, "withdrawal request failed", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "withdrawal requested successfully", res)
}

// ListTransactions: GET /api/wallets/transactions
func (h *PaymentHandler) ListTransactions(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		return response.Unauthorized(c, "authentication required")
	}

	txType := c.Query("type", "ALL")
	status := c.Query("status", "ALL")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	txs, total, err := h.service.ListTransactions(c.Context(), userID, txType, status, limit, offset)
	if err != nil {
		return response.Error(c, fiber.StatusInternalServerError, "failed to list transactions", err.Error())
	}

	return response.Success(c, fiber.StatusOK, "transactions retrieved", fiber.Map{
		"items":  txs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Webhook: POST /api/payments/razorpay/webhook
func (h *PaymentHandler) Webhook(c *fiber.Ctx) error {
	sig := c.Get("X-Razorpay-Signature")
	body := c.Body()

	if err := h.service.HandleWebhook(c.Context(), body, sig); err != nil {
		return response.BadRequest(c, err.Error())
	}

	return response.Success(c, fiber.StatusOK, "webhook processed", nil)
}
