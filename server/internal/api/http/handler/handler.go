package handler

import (
	"context"

	"github.com/arcaptcha/kaftar/server/internal/app"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

type handler struct {
	app  app.App
	echo *echo.Echo
}

func RegisterApi(a app.App, e *echo.Echo) {
	h := &handler{app: a, echo: e}
	h.Register()
}

func (h *handler) Register() {
	h.echo.GET("/health", h.health)

	api := h.echo.Group("/api/v1")

	api.POST("/sms/send", h.sendSMS)
	api.GET("/sms/status", h.statusSMS)

	api.POST("/email/send", h.sendEmail)
	api.GET("/email/status", h.statusEmail)

	api.POST("/mattermost/send", h.sendMattermost)
	api.GET("/mattermost/status", h.statusMattermost)

	api.POST("/bale/send", h.SendBale)
	api.GET("/bale/status", h.StatusBale)

	api.POST("/http/send", h.sendHTTP)
	api.GET("/http/status", h.statusHTTP)
}

func (h *handler) service() service.Service {
	return h.app.Service(context.Background())
}
