package handler

import (
	"net/http"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

// SendBale sends a Bale message
func (h *handler) SendBale(c echo.Context) error {
	var req dto.BaleMessage
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
			Details: map[string]any{"error": err},
		})
	}
	sendAt, err := parseQueryParamTime(c, "send_at")
	if err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to parse send_at; must be RFC3339",
			Details: map[string]any{"error": err},
		})
	}
	entityMsg, err := req.ToEntity()
	if err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to process attachments",
			Details: map[string]any{"error": err.Error()},
		})
	}
	opts := &service.SendOptions{
		MaxRetries:     maxRetries,
		IdempotencyKey: c.Request().Header.Get("Idempotency-Key"),
		SendAt:         sendAt,
	}
	id, err := h.service().Send(c.Request().Context(), entityMsg, opts)
	if err != nil {
		return sendSubmissionError(c, err)
	}
	return c.JSON(http.StatusOK, &dto.SendBaleResult{ID: id})
}

// StatusBale retrieves the status of a Bale message
func (h *handler) StatusBale(c echo.Context) error {
	var req dto.StatusBaleRequest
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
			Message: "Message ID is required",
		})
	}
	status, err := h.service().Status(c.Request().Context(), req.ID)
	if err != nil {
		return sendStatusError(c, "Failed to get Bale message status", err)
	}
	return c.JSON(http.StatusOK, messageStatus(status))
}
