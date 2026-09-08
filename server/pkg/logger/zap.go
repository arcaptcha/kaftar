package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Mode string

const (
	ModeProduction  Mode = "production"
	ModeDevelopment Mode = "development"
)

func (m Mode) orDefault() {
	switch m {
	case ModeDevelopment, ModeProduction:
	default:
		m = ModeProduction
	}
}

func NewZapLogger(mode Mode) *zap.Logger {
	mode.orDefault()
	logLevel := zap.InfoLevel
	if mode == ModeDevelopment {
		logLevel = zap.DebugLevel
	}

	level := zap.NewAtomicLevelAt(logLevel)
	consoleEncoderCfg := getConsoleEncoderConfig()

	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderCfg)

	stdout := zapcore.AddSync(os.Stdout)

	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, stdout, level),
	)

	opts := []zap.Option{zap.AddCaller()}
	if mode == ModeDevelopment {
		opts = append(opts, zap.Development())
	}

	return zap.New(core, opts...)
}

func getConsoleEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.TimeKey = "timestamp"
	cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	return cfg
}
