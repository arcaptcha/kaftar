package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arcaptcha/kaftar/server/internal/config"
	"github.com/arcaptcha/kaftar/server/internal/entity"
	"github.com/arcaptcha/kaftar/server/internal/repository"
	"github.com/arcaptcha/kaftar/server/internal/service"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type testApp struct{ service service.Service }

func (application testApp) Config() config.Config                   { return config.Config{} }
func (application testApp) Logger() *zap.Logger                     { return zap.NewNop() }
func (application testApp) Service(context.Context) service.Service { return application.service }
func (application testApp) Shutdown(context.Context) error          { return nil }

type testService struct {
	message entity.Message
	options service.SendOptions
	err     error
}

func (backend *testService) Shutdown(context.Context) error { return nil }
func (backend *testService) BeginShutdown()                 {}

func (backend *testService) Send(_ context.Context, message entity.Message, options *service.SendOptions) (string, error) {
	backend.message = message
	backend.options = *options
	return "01900000-0000-7000-8000-000000000001", backend.err
}

func (backend *testService) Status(_ context.Context, id string) (entity.OutboxStatus, error) {
	return entity.OutboxStatus{ID: uuid.MustParse(id), Channel: entity.ChannelSMS, State: entity.OutboxStatePending}, backend.err
}

func TestHTTPContract(test *testing.T) {
	backend := &testService{}
	router := echo.New()
	RegisterApi(testApp{service: backend}, router)
	channels := []string{"sms", "email", "mattermost", "bale", "http"}
	for _, channel := range channels {
		test.Run(channel, func(test *testing.T) {
			backend.err = nil
			backend.message = nil
			request := httptest.NewRequest(http.MethodPost, "/api/v1/"+channel+"/send?max_retries=-1&send_at=2030-01-01T00:00:00Z", strings.NewReader(`{}`))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			request.Header.Set("Idempotency-Key", "request-123")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || backend.message == nil || backend.message.Channel().String() != channel {
				test.Fatalf("send response = %d %s, message = %v", response.Code, response.Body, backend.message)
			}
			if backend.options.IdempotencyKey != "request-123" || backend.options.MaxRetries != -1 || backend.options.SendAt == nil || backend.options.SendAt.Year() != 2030 {
				test.Fatalf("options = %+v", backend.options)
			}
			var result map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["id"] == "" {
				test.Fatalf("send body = %s", response.Body)
			}
			for _, query := range []string{"max_retries=bad", "send_at=bad"} {
				request := httptest.NewRequest(http.MethodPost, "/api/v1/"+channel+"/send?"+query, strings.NewReader(`{}`))
				request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusBadRequest {
					test.Fatalf("%s: %d", query, response.Code)
				}
			}
			backend.err = service.ErrSenderUnavailable
			for _, scenario := range []struct {
				err    error
				status int
			}{{repository.ErrIdempotencyConflict, http.StatusConflict}, {service.ErrInvalidSubmission, http.StatusBadRequest}, {service.ErrShuttingDown, http.StatusServiceUnavailable}} {
				backend.err = scenario.err
				request := httptest.NewRequest(http.MethodPost, "/api/v1/"+channel+"/send", strings.NewReader(`{}`))
				request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != scenario.status {
					test.Fatalf("submission error status = %d, want %d", response.Code, scenario.status)
				}
			}
			backend.err = service.ErrSenderUnavailable
			request = httptest.NewRequest(http.MethodPost, "/api/v1/"+channel+"/send", strings.NewReader(`{}`))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable {
				test.Fatalf("disabled sender = %d", response.Code)
			}
			backend.err = nil
			request = httptest.NewRequest(http.MethodGet, "/api/v1/"+channel+"/status?id="+result["id"], nil)
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status_text":"pending"`) {
				test.Fatalf("status = %d %s", response.Code, response.Body)
			}
			backend.err = repository.ErrRecordNotFound
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound {
				test.Fatalf("missing record = %d", response.Code)
			}
		})
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `"Ok"` {
		test.Fatalf("health = %d %s", response.Code, response.Body)
	}
}

func TestQueryTimeDefaults(test *testing.T) {
	router := echo.New()
	for _, query := range []string{"", "send_at=2026-01-01T12:00:00%2B03:30"} {
		ctx := router.NewContext(httptest.NewRequest(http.MethodGet, "/?"+query, nil), httptest.NewRecorder())
		value, err := parseQueryParamTime(ctx, "send_at")
		if err != nil {
			test.Fatal(err)
		}
		if query == "" && value != nil {
			test.Fatal("unset time must be nil")
		}
		if query != "" && !value.Equal(time.Date(2026, 1, 1, 8, 30, 0, 0, time.UTC)) {
			test.Fatalf("time = %v", value)
		}
	}
}
