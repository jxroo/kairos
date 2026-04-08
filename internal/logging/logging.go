package logging

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(level, format, logDir string) (*zap.Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	lvl, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("parsing log level %q: %w", level, err)
	}

	logFile := filepath.Join(logDir, "kairos.log")
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	var encoder zapcore.Encoder
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	if format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, zapcore.AddSync(file), lvl),
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), lvl),
	)

	return zap.New(core), nil
}
