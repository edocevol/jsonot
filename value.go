package jsonot

import (
	"encoding/json"
	"fmt"

	"github.com/samber/mo"
)

// NodeType 定义了 JSON Object 的节点类型
type NodeType int

// Format implements fmt.Formatter interface for NodeType
func (nt NodeType) Format(st fmt.State, verb rune) {
	switch nt {
	case Null:
		_, _ = fmt.Fprint(st, "null")
	case Numeric:
		_, _ = fmt.Fprint(st, "numeric")
	case String:
		_, _ = fmt.Fprint(st, "string")
	case Bool:
		_, _ = fmt.Fprint(st, "bool")
	case Array:
		_, _ = fmt.Fprint(st, "array")
	case Object:
		_, _ = fmt.Fprint(st, "object")
	case Any:
		_, _ = fmt.Fprint(st, "any")
	default:
		_, _ = fmt.Fprint(st, "unknown")
	}
}

const (
	// Null is reflection of nil.(interface{})
	Null NodeType = iota
	// Numeric is reflection of float64
	Numeric
	// String is reflection of string
	String
	// Bool is reflection of bool
	Bool
	// Array is reflection of []*Value
	Array
	// Object is reflection of map[string]*Value
	Object
	// Any is reflection of any
	Any
)

// ApplierOperator 定义了应用操作的接口
type ApplierOperator interface {
	// ApplyOnValue 应用操作到 Value 上
	ApplyOnValue(val Value, paths Path) mo.Result[Value]
}

// Value 定义了 JSON Object 的节点接口
type Value interface {
	Format(st fmt.State, verb rune)
	// Type 返回节点的类型
	Type() NodeType
	// IsBool 返回节点是否为布尔类型
	IsBool() bool
	// IsNull 返回节点是否为 null 类型
	IsNull() bool
	// IsNumeric 返回节点是否为数字类型
	IsNumeric() bool
	// IsInt 返回节点是否为整数类型
	IsInt() bool
	// IsString 返回节点是否为字符串类型
	IsString() bool
	// IsArray 返回节点是否为数组类型
	IsArray() bool
	// IsObject 返回节点是否为对象类型
	IsObject() bool
	// Unmarshal 实现 JSON 的解码
	Unmarshal(data []byte) error
	// PackAny 解析任意类型的数据, sonic 类型的实现需要此方法
	PackAny()
	// GetKey 返回节点的键名，如果是对象类型
	GetKey(key string) mo.Option[Value]
	// GetByPath 返回节点的键名，如果是对象类型
	GetByPath(path ...any) mo.Option[Value]
	// GetStringKey 返回节点的键名，如果是对象类型
	GetStringKey(key string) mo.Option[string]
	// GetIntKey 返回节点的键名，如果是对象类型
	GetIntKey(key string) mo.Option[int]
	// GetArray 返回节点的数组，如果是数组类型
	GetArray() mo.Result[[]Value]
	// GetMap 返回节点的对象，如果是对象类型
	GetMap() mo.Result[map[string]Value]
	// GetNumeric 返回节点的数字值，如果是数字类型
	GetNumeric() mo.Result[float64]
	// GetInt 返回节点的整数值，如果是数字类型
	GetInt() mo.Result[int]
	// GetString 返回节点的字符串值，如果是字符串类型
	GetString() mo.Result[string]
	// SetKey 设置对象中的键名，如果是对象类型
	SetKey(key string, value Value) error
	// DeleteKey 删除对象中的键名，如果是对象类型
	DeleteKey(key string) error
	// HasKey 检查节点是否包含指定的键名，如果是对象类型
	HasKey(key string) bool
	// Size 返回对象的属性数量
	Size() int
	// Equals 检查两个节点是否相等
	Equals(other Value) bool
	// RawMessage 返回节点的原始消息
	RawMessage() json.RawMessage
}

// ValueBrian 定义了 value 的血统信息
type ValueBrian struct {
	KeyInParent   string      // 当前节点在父节点中的键名
	IndexInParent int         // 当前节点在父节点中的索引
	Value         Value       // 当前节点的值
	Parent        *ValueBrian // 当前节点的父节点
}
