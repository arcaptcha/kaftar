package handler

import (
	"net/http"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

// SendHTTP sends an HTTP request.
func (h *handler) sendHTTP(c echo.Context) error {
	var req dto.HTTPMessage
	if err := c.Bind(&req); err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Invalid request payload",
			Details: map[string]any{"error": err.Error()},
		})
	}
	maxRetries, err := parseQueryParamInt(c, "max_retries")
	if err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse query param",
			Details: map[string]any{"error": err.Error()},
		})
	}
	sendAt, err := parseQueryParamTime(c, "send_at")
	if err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse send_at; must be RFC3339",
			Details: map[string]any{"error": err.Error()},
		})
	}
	id, err := h.service().Send(c.Request().Context(), req.ToEntity(), &service.SendOptions{MaxRetries: maxRetries, SendAt: sendAt, IdempotencyKey: c.Request().Header.Get("Idempotency-Key")})
	if err != nil {
		return sendSubmissionError(c, err)
	}
	return c.JSON(http.StatusOK, &dto.SendHTTPResult{ID: id})
}

// GetHTTPStatus retrieves the status of an HTTP request.
func (h *handler) statusHTTP(c echo.Context) error {
	var req dto.StatusHTTPRequest
	if err := c.Bind(&req); err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Invalid request parameters",
			Details: map[string]any{"error": err.Error()},
		})
	}
	if req.ID == "" {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "HTTP request ID is required",
		})
	}
	status, err := h.service().Status(c.Request().Context(), req.ID)
	if err != nil {
		return sendStatusError(c, "Failed to get HTTP request status", err)
	}
	return c.JSON(http.StatusOK, messageStatus(status))
}
