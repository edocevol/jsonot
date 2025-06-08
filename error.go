package jsonot

import "errors"

var (
	// ErrUnexpectedError 未知错误
	ErrUnexpectedError = errors.New("unexpected error")
	// ErrInvalidParameter 参数无效
	ErrInvalidParameter = errors.New("invalid parameter")
	// ErrInvalidOperation 操作无效
	ErrInvalidOperation = errors.New("invalid operation")
	// ErrInvalidPathFormat 路径格式无效
	ErrInvalidPathFormat = errors.New("invalid path format")
	// ErrInvalidPathElement 路径元素无效
	ErrInvalidPathElement = errors.New("invalid path element")
	// ErrBadPath 路径错误
	ErrBadPath = errors.New("bad path")
	// ErrSerdeError 序列化或反序列化错误
	ErrSerdeError = errors.New("invalid JSON key or value")
	// ErrConflictSubType 子类型名称冲突
	ErrConflictSubType = errors.New("sub type name conflict with internal sub type name")
)
