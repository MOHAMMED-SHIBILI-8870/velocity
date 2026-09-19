package middleware

import (
	"context"
	"strings"

	"velocity/internal/service/userservice"
	"velocity/internal/transport/grpc/client/identity"

	"github.com/gofiber/fiber/v2"
)

type AuthMiddleware struct {
	identityClient *identity.Client
	userService    *userservice.Service
}

func NewAuthMiddleware(
	identityClient *identity.Client,
	userService *userservice.Service,
) *AuthMiddleware {
	return &AuthMiddleware{
		identityClient: identityClient,
		userService:    userService,
	}
}

func (m *AuthMiddleware) ensureUserProvisioned(ctx context.Context, userID int64, email string) {
	if m.userService != nil && userID > 0 {
		_, _ = m.userService.CreateUser(ctx, userservice.CreateUserRequest{
			ID:    userID,
			Email: email,
		})
	}
}

func (m *AuthMiddleware) Authenticate(c *fiber.Ctx) error {

	// -------------------------
	// Read Authorization Header
	// -------------------------

	authHeader := c.Get("Authorization")

	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "missing authorization header",
		})
	}

	// -------------------------
	// Check Bearer Token
	// -------------------------

	const bearer = "Bearer "

	if !strings.HasPrefix(authHeader, bearer) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid authorization header",
		})
	}

	token := strings.TrimPrefix(authHeader, bearer)

	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "missing token",
		})
	}

	// -------------------------
	// Validate Token
	// -------------------------

	resp, err := m.identityClient.ValidateToken(
		c.Context(),
		token,
	)

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "authentication service unavailable",
		})
	}

	if !resp.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": resp.Error,
		})
	}

	// -------------------------
	// Store User Context & Ensure Provisioned
	// -------------------------

	userID := int64(resp.UserId)
	m.ensureUserProvisioned(c.Context(), userID, resp.Email)

	c.Locals("authUser", &AuthenticatedUser{
		UserID: userID,
		Email:  resp.Email,
		Role:   resp.Role,
	})

	return c.Next()
}

func (m *AuthMiddleware) OptionalAuthenticate(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	const bearer = "Bearer "
	if authHeader == "" || !strings.HasPrefix(authHeader, bearer) {
		return c.Next()
	}

	token := strings.TrimPrefix(authHeader, bearer)
	if token == "" {
		return c.Next()
	}

	resp, err := m.identityClient.ValidateToken(c.Context(), token)
	if err == nil && resp != nil && resp.Valid {
		userID := int64(resp.UserId)
		m.ensureUserProvisioned(c.Context(), userID, resp.Email)

		c.Locals("authUser", &AuthenticatedUser{
			UserID: userID,
			Email:  resp.Email,
			Role:   resp.Role,
		})
	}

	return c.Next()
}

func (m *AuthMiddleware) AuthenticateWS(c *fiber.Ctx) error {

	token := c.Query("token")

	if token == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "missing token",
		})
	}

	resp, err := m.identityClient.ValidateToken(
		c.Context(),
		token,
	)

	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "authentication service unavailable",
		})
	}

	if !resp.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": resp.Error,
		})
	}

	userID := int64(resp.UserId)
	m.ensureUserProvisioned(c.Context(), userID, resp.Email)

	c.Locals("authUser", &AuthenticatedUser{
		UserID: userID,
		Email:  resp.Email,
		Role:   resp.Role,
	})

	return c.Next()
}

