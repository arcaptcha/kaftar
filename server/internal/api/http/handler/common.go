package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/api/dto"
	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/labstack/echo/v4"
)

// health godoc
func (h *handler) health(c echo.Context) error {
	return c.JSON(http.StatusOK, "Ok")
}

func sendError(c echo.Context, e *dto.Error) error {
	return c.JSON(e.Status, e)
}

func sendStatusError(c echo.Context, message string, err error) error {
	status := http.StatusInternalServerError
	if errors.Is(err, repository.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	return sendError(c, &dto.Error{
		Status:  status,
		Message: message,
		Details: map[string]any{"error": err.Error()},
	})
}

func messageStatus(status entity.OutboxStatus) dto.MessageStatus {
	return dto.NewMessageStatus(status)
}

func sendSubmissionError(ctx echo.Context, err error) error {
	status, message := http.StatusInternalServerError, "Submission could not be saved"
	switch {
	case errors.Is(err, repository.ErrIdempotencyConflict):
		status, message = http.StatusConflict, "Idempotency key conflicts with an existing submission"
	case errors.Is(err, service.ErrSenderUnavailable), errors.Is(err, service.ErrShuttingDown):
		status, message = http.StatusServiceUnavailable, "Sender unavailable"
	case errors.Is(err, service.ErrInvalidSubmission):
		status, message = http.StatusBadRequest, "Invalid submission"
	}
	return sendError(ctx, &dto.Error{Status: status, Message: message})
}

func parseQueryParamInt(c echo.Context, name string) (int, error) {
	valueStr := c.QueryParam(name)
	if valueStr == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func parseQueryParamTime(c echo.Context, name string) (*time.Time, error) {
	valueStr := c.QueryParam(name)
	if valueStr == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, valueStr)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
