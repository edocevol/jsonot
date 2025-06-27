package jsonot

import (
	"encoding/json"

	"github.com/bytedance/sonic/ast"
	"github.com/spyzhov/ajson"
)

// PrimitiveType 定义了支持的原始类型
type PrimitiveType interface {
	string | int | int64 | float64 | bool
}

// ValueFactory 接口定义了一个工厂方法，用于从 JSON 数据反序列化为 Value 类型
type ValueFactory interface {
	// Unmarshal unmarshal JSON data into a Value
	Unmarshal([]byte) (Value, error)
	// FromPrimitive create a primitive JSON Object from a primitive type
	FromPrimitive(data any) Value
	// FromAny create a new JSON Object node from any type
	FromAny(any) Value
	// FromArray create a new JSON Array node from a slice of Value
	FromArray([]Value) Value
}

var (
	_ ValueFactory = (*AJSONValueFactory)(nil)
	_ ValueFactory = (*SonicValueFactory)(nil)
)

// defaultValueFactory 是一个默认的 ValueFactory 实例
var defaultValueFactory ValueFactory = NewSonicValueFactory()

// UnmarshalValue 创建一个新的 JSON Object 节点
func UnmarshalValue(data []byte) (Value, error) {
	return defaultValueFactory.Unmarshal(data)
}

// ValueFromPrimitive 创建一个新的 JSON Object 节点
func ValueFromPrimitive[T string | int | int64 | float64](data T) Value {
	return defaultValueFactory.FromPrimitive(data)
}

// ValueFromAny 创建一个新的 JSON Object 节点
func ValueFromAny(data any) Value {
	return defaultValueFactory.FromAny(data)
}

// ValueFromArray 创建一个新的 JSON Array 节点
func ValueFromArray(arr []Value) Value {
	return defaultValueFactory.FromArray(arr)
}

// AJSONValueFactory 用于创建 AJSONValue 的工厂
type AJSONValueFactory struct{}

// NewAJSONValueFactory 创建一个新的 AJSONValueFactory 实例
func NewAJSONValueFactory() ValueFactory {
	return &AJSONValueFactory{}
}

// Unmarshal 实现了 ValueFactory 接口的 Unmarshal 方法
func (vf AJSONValueFactory) Unmarshal(bytes []byte) (Value, error) {
	node, err := ajson.Unmarshal(bytes)
	if err != nil {
		return nil, err
	}
	return NewAJSONValue(node), nil
}

// FromPrimitive 创建一个新的 JSON Object 节点
func (vf AJSONValueFactory) FromPrimitive(pt any) Value {
	var node *ajson.Node
	switch v := any(pt).(type) {
	case string:
		node = ajson.StringNode("", v)
	case int:
		node = ajson.NumericNode("", float64(v))
	case int64:
		node = ajson.NumericNode("", float64(v))
	case float64:
		node = ajson.NumericNode("", v)
	case bool:
		node = ajson.BoolNode("", v)
	}
	return NewAJSONValue(node)
}

// FromAny 创建一个新的 JSON Object 节点
func (vf AJSONValueFactory) FromAny(a any) Value {
	bytes, _ := json.Marshal(a)
	node, _ := ajson.Unmarshal(bytes)
	if node == nil {
		node = ajson.NullNode("")
	}

	return NewAJSONValue(node)
}

// FromArray 创建一个新的 JSON Array 节点
func (vf AJSONValueFactory) FromArray(values []Value) Value {
	var nodes []*ajson.Node
	for k := range values {
		an, _ := values[k].(*AJSONValue) // must be AJSONValue
		nodes = append(nodes, an.node.Clone())
	}

	node := ajson.ArrayNode("", nodes)
	return NewAJSONValue(node)
}

// SonicValueFactory 用于创建 SonicValue 的工厂
type SonicValueFactory struct{}

// NewSonicValueFactory 创建一个新的 SonicValueFactory 实例
func NewSonicValueFactory() ValueFactory {
	return &SonicValueFactory{}
}

// Unmarshal 实现了 ValueFactory 接口的 Unmarshal 方法
func (vf SonicValueFactory) Unmarshal(bytes []byte) (Value, error) {
	var node ast.Node
	if err := node.UnmarshalJSON(bytes); err != nil {
		return nil, err
	}
	return NewSonicValue(&node), nil
}

// FromPrimitive 创建一个新的 JSON Object 节点
func (vf SonicValueFactory) FromPrimitive(data any) Value {
	var node ast.Node
	switch v := data.(type) {
	case string:
		node = ast.NewString(v)
	case bool:
		node = ast.NewBool(v)
	case int, int64, float64:
		node = ast.NewAny(v)
	default:
		node = ast.NewNull() // 默认处理为 null
	}
	return NewSonicValue(&node)
}

// FromAny 创建一个新的 JSON Object 节点
func (vf SonicValueFactory) FromAny(data any) Value {
	var node ast.Node
	bytes, _ := json.Marshal(data)
	if err := node.UnmarshalJSON(bytes); err != nil {
		node = ast.NewNull()
	}

	value := NewSonicValue(&node)
	return value
}

// FromArray 创建一个新的 JSON Array 节点
func (vf SonicValueFactory) FromArray(values []Value) Value {
	var nodes []ast.Node
	for k := range values {
		sn, _ := values[k].(*SonicValue) // must be SonicValue
		nodes = append(nodes, *sn.node)
	}

	node := ast.NewArray(nodes)
	value := NewSonicValue(&node)
	return value
}
