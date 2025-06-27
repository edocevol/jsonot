package jsonot

import (
	"fmt"
)

var (
	// enableDebugLog 是否启用调试日志
	enableDebugLog = false
	// log 日志记录器
	log Logger = &FmtLogger{}
)

// SetEnableDebug 控制是否启用调试日志
func SetEnableDebug() {
	enableDebugLog = true
}

// SetLogger 设置日志记录器
func SetLogger(l Logger) {
	log = l
}

// Logger 日志接口
type Logger interface {
	// Debugf 打印调试日志
	Debugf(format string, args ...interface{})
	// ContextDebugf 打印上下文调试日志
	ContextDebugf(ctx interface{}, format string, args ...interface{})
	// Errorf 打印错误日志
	Errorf(format string, args ...interface{})
	// ContextErrorf 打印上下文错误日志
	ContextErrorf(ctx interface{}, format string, args ...interface{})
}

var _ Logger = (*FmtLogger)(nil)

// FmtLogger 实现 Logger 接口的日志记录器
type FmtLogger struct{}

// Debugf 打印调试日志
func (f FmtLogger) Debugf(format string, args ...interface{}) {
	if !enableDebugLog {
		return
	}
	fmt.Printf(fmt.Sprintf("\033[36m[JSONOT]\033[0m--> %s", format), args...)
}

// ContextDebugf 打印上下文调试日志
func (f FmtLogger) ContextDebugf(_ interface{}, format string, args ...interface{}) {
	if !enableDebugLog {
		return
	}
	fmt.Printf(fmt.Sprintf("\033[36m[JSONOT]\033[0m--> %s", format), args...)
}

// Errorf 打印错误日志
func (f FmtLogger) Errorf(format string, args ...interface{}) {
	fmt.Printf(fmt.Sprintf("\033[31m[JSONOT]\033[0m--> %s", format), args...)
}

// ContextErrorf 打印上下文错误日志
func (f FmtLogger) ContextErrorf(_ interface{}, format string, args ...interface{}) {
	fmt.Printf(fmt.Sprintf("\033[31m[JSONOT]\033[0m--> %s", format), args...)
}
