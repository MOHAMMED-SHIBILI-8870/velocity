package router

import (
	"github.com/gofiber/fiber/v2"

	"velocity/internal/transport/http/handler"
)

func RegisterPositionRoutes(
	api fiber.Router,
	positionHandler *handler.PositionHandler,
	auth fiber.Handler,
) {
	positions := api.Group("/positions", auth)

	positions.Get("/", positionHandler.List)

	positions.Get("/:symbol", positionHandler.GetBySymbol)

}
