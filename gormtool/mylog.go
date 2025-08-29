// gormtool\mylog.go
package gormtool

import (
	"context"
	"encoding/json"
	"fmt"

	"log"
	"os"
)

// Logger 接口
type Logger interface {
	Debug(ctx context.Context, msg string, fields map[string]interface{})
	Info(ctx context.Context, msg string, fields map[string]interface{})
	Warn(ctx context.Context, msg string, fields map[string]interface{})
	Error(ctx context.Context, msg string, fields map[string]interface{})
}

// DefaultLogger 默认日志实现
type DefaultLogger struct {
	logger *log.Logger
}

func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		logger: log.New(os.Stdout, "[GORMTOOL] ", log.LstdFlags|log.Lshortfile),
	}
}

func (l *DefaultLogger) Debug(ctx context.Context, msg string, fields map[string]interface{}) {
	l.log("DEBUG", msg, fields)
}

func (l *DefaultLogger) Info(ctx context.Context, msg string, fields map[string]interface{}) {
	l.log("INFO", msg, fields)
}

func (l *DefaultLogger) Warn(ctx context.Context, msg string, fields map[string]interface{}) {
	l.log("WARN", msg, fields)
}

func (l *DefaultLogger) Error(ctx context.Context, msg string, fields map[string]interface{}) {
	l.log("ERROR", msg, fields)
}

func (l *DefaultLogger) log(level, msg string, fields map[string]interface{}) {
	logMsg := fmt.Sprintf("[%s] %s", level, msg)
	if len(fields) > 0 {
		jsonFields, _ := json.Marshal(fields)
		logMsg += " " + string(jsonFields)
	}
	l.logger.Output(3, logMsg)
}
