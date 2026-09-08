package handler

import (
	"net/http"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

// SendMattermost sends a Mattermost message
func (h *handler) sendMattermost(c echo.Context) error {
	var req dto.MattermostMessage
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
	opts := &service.SendOptions{
		MaxRetries:     maxRetries,
		IdempotencyKey: c.Request().Header.Get("Idempotency-Key"),
		SendAt:         sendAt,
	}
	entityMsg, err := req.ToEntity()
	if err != nil {
		return sendError(c, &dto.Error{
			Status:  http.StatusBadRequest,
			Message: "Failed to process attachments",
			Details: map[string]any{"error": err.Error()},
		})
	}
	id, err := h.service().Send(c.Request().Context(), entityMsg, opts)
	if err != nil {
		return sendSubmissionError(c, err)
	}
	return c.JSON(http.StatusOK, &dto.SendMattermostResult{ID: id})
}

// GetMattermostStatus retrieves the status of a Mattermost message
func (h *handler) statusMattermost(c echo.Context) error {
	var req dto.StatusMattermostRequest
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
			Message: "Mattermost message ID is required",
		})
	}
	status, err := h.service().Status(c.Request().Context(), req.ID)
	if err != nil {
		return sendStatusError(c, "Failed to get Mattermost message status", err)
	}
	return c.JSON(http.StatusOK, messageStatus(status))
}
