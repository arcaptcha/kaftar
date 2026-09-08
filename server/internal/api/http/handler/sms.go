package handler

import (
	"net/http"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

// SendSMS sends an SMS message
func (h *handler) sendSMS(c echo.Context) error {
	var req dto.SMSMessage
	if err := c.Bind(&req); err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to bind SMS message",
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
	opts := &service.SendOptions{
		MaxRetries:     maxRetries,
		IdempotencyKey: c.Request().Header.Get("Idempotency-Key"),
		SendAt:         sendAt,
	}
	id, err := h.service().Send(c.Request().Context(), req.ToEntity(), opts)
	if err != nil {
		return sendSubmissionError(c, err)
	}
	return c.JSON(http.StatusOK, &dto.SendSMSResult{ID: id})
}

// GetSMSStatus retrieves the status of an SMS message
func (h *handler) statusSMS(c echo.Context) error {
	var req dto.StatusSMSRequest
	if err := c.Bind(&req); err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to bind SMS status request",
			Details: map[string]any{"error": err.Error()},
		})
	}
	status, err := h.service().Status(c.Request().Context(), req.ID)
	if err != nil {
		return sendStatusError(c, "Failed to get SMS status", err)
	}
	return c.JSON(http.StatusOK, messageStatus(status))
}
