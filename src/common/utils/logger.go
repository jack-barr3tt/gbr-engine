package utils

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sharedLogger *zap.SugaredLogger

func InitLogger() {
	if sharedLogger != nil {
		return
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "L",
		MessageKey:     "M",
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.0000"),
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	level := zapcore.InfoLevel
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		if parsedLevel, err := zapcore.ParseLevel(lvl); err == nil {
			level = parsedLevel
		}
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		level,
	)

	logger := zap.New(core, zap.AddCallerSkip(1))
	sharedLogger = logger.Sugar()
}

func GetLogger() *zap.SugaredLogger {
	if sharedLogger == nil {
		InitLogger()
	}
	return sharedLogger
}

func SyncLogger() {
	if sharedLogger != nil {
		_ = sharedLogger.Sync()
	}
}
