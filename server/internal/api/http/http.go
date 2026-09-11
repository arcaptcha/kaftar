package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/arcaptcha/kaftar/server/internal/api/http/handler"
	"github.com/arcaptcha/kaftar/server/internal/app"
	"github.com/labstack/echo/v4"
)

var ErrServerClosed = http.ErrServerClosed

type Server interface {
	Start() error
	Shutdown(ctx context.Context) error
}

type httpServer struct {
	echo *echo.Echo
	app  app.App
}

func NewServer(app app.App) Server {
	server := &httpServer{
		echo: echo.New(),
		app:  app,
	}
	server.echo.Use(app.Metrics().Middleware)
	return server
}

func (s *httpServer) Start() error {
	handler.RegisterApi(s.app, s.echo)
	addr := fmt.Sprintf(":%d", s.app.Config().Http.Port)
	return s.echo.Start(addr)
}

func (s *httpServer) Shutdown(ctx context.Context) error {
	s.app.Service(ctx).BeginShutdown()
	httpErr := s.echo.Shutdown(ctx)
	return errors.Join(httpErr, s.app.Shutdown(ctx))
}
