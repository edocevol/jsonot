package jsonot

import (
	"encoding/json"
	"fmt"

	"github.com/samber/mo"
	"github.com/spyzhov/ajson"
)

// NewAJSONValue 创建一个新的 AJSONValue
func NewAJSONValue(node *ajson.Node) *AJSONValue {
	return &AJSONValue{node: node}
}

var _ Value = (*AJSONValue)(nil)

// AJSONValue 定义了 JSON Object 的节点
type AJSONValue struct {
	node *ajson.Node
}

// Type returns the type of the JSON node
func (n *AJSONValue) Type() ValueType {
	switch n.node.Type() {
	case ajson.Null:
		return Null
	case ajson.String:
		return String
	case ajson.Numeric:
		return Numeric
	case ajson.Bool:
		return Bool
	case ajson.Object:
		return Object
	case ajson.Array:
		return Array
	default:
		return Object
	}
}

// IsBool 返回节点是否为布尔类型
func (n *AJSONValue) IsBool() bool {
	return n.node.IsBool()
}

// IsNull 返回节点是否为 null 类型
func (n *AJSONValue) IsNull() bool {
	return n.node.IsNull()
}

// IsNumeric 返回节点是否为数字类型
func (n *AJSONValue) IsNumeric() bool {
	return n.node.IsNumeric()
}

// IsInt 返回节点是否为整数类型
func (n *AJSONValue) IsInt() bool {
	val, err := n.node.Value()
	if err != nil {
		return false
	}
	switch v := val.(type) {
	case float64:
		return v == float64(int(v)) // Check if the float64 can be represented as an int
	case int, int64:
		return true
	}

	return false
}

// IsString returns true if the node is a string type
func (n *AJSONValue) IsString() bool {
	return n.node.IsString()
}

// IsArray returns true if the node is an array type
func (n *AJSONValue) IsArray() bool {
	return n.node.IsArray()
}

// IsObject returns true if the node is an object type
func (n *AJSONValue) IsObject() bool {
	return n.node.IsObject()
}

// Unmarshal implements JSON decoding
func (n *AJSONValue) Unmarshal(data []byte) error {
	node, err := ajson.Unmarshal(data)
	if err != nil {
		return NewError(UnexpectedError).Wrap(err)
	}

	n.node = node
	return nil
}

// PackAny parses any type of data into the JSON node
func (n *AJSONValue) PackAny() {
}

// GetKey retrieves a key from the JSON object
func (n *AJSONValue) GetKey(key string) mo.Option[Value] {
	node, err := n.node.GetKey(key)
	if err != nil {
		return mo.Option[Value]{}
	}

	return mo.Some[Value](NewAJSONValue(node))
}

// GetByPath retrieves a value by path from the JSON object
func (n *AJSONValue) GetByPath(path ...any) mo.Option[Value] {
	var node *ajson.Node
	for _, p := range path {
		var err error
		switch v := p.(type) {
		case string:
			node, err = n.node.GetKey(v)
		case int:
			node, err = n.node.GetIndex(v)
		}
		if err != nil {
			return mo.None[Value]()
		}
	}

	newNode := NewAJSONValue(node)
	return mo.Some[Value](newNode)
}

// GetStringKey retrieves a string key from the JSON object
func (n *AJSONValue) GetStringKey(key string) mo.Option[string] {
	node, err := n.node.GetKey(key)
	if err != nil {
		return mo.None[string]()
	}

	val, err := node.Value()
	if err != nil {
		return mo.None[string]()
	}

	if str, ok := val.(string); ok {
		return mo.Some[string](str)
	}

	return mo.None[string]()
}

// GetIntKey retrieves an integer key from the JSON object
func (n *AJSONValue) GetIntKey(key string) mo.Option[int] {
	node, err := n.node.GetKey(key)
	if err != nil {
		return mo.None[int]()
	}

	newNode := NewAJSONValue(node)
	if newNode.IsInt() {
		val := newNode.GetInt()
		if val.IsError() {
			return mo.None[int]()
		}
		return mo.Some[int](val.MustGet())
	}

	return mo.None[int]()
}

// GetArray retrieves the array from the JSON node
func (n *AJSONValue) GetArray() mo.Result[[]Value] {
	nodes, err := n.node.GetArray()
	if err != nil {
		return mo.Err[[]Value](err)
	}

	var result []Value
	for _, item := range nodes {
		result = append(result, NewAJSONValue(item))
	}

	return mo.Ok(result)
}

// UpdateArray 从传入的 Value 数组更新当前 Value
func (n *AJSONValue) UpdateArray(values []Value) error {
	var nodes []*ajson.Node
	for _, v := range values {
		if av, ok := v.(*AJSONValue); ok {
			nodes = append(nodes, av.node.Clone())
		} else {
			return fmt.Errorf("value is not an AJSONValue")
		}
	}

	if err := n.node.SetArray(nodes); err != nil {
		return fmt.Errorf("failed to update array: %w", err)
	}

	return nil
}

// UpdateObject 从传入的 Value 更新当前 Value
func (n *AJSONValue) UpdateObject(values Value) error {
	mapValues, ok := values.(*AJSONValue)
	if !ok {
		return fmt.Errorf("value is not an AJSONValue")
	}

	n.node = mapValues.node
	return nil
}

// GetMap retrieves the map from the JSON node
func (n *AJSONValue) GetMap() mo.Result[map[string]Value] {
	properties, err := n.node.GetObject()
	if err != nil {
		return mo.Err[map[string]Value](err)
	}

	result := make(map[string]Value, len(properties))
	for key, item := range properties {
		result[key] = NewAJSONValue(item)
	}

	return mo.Ok(result)
}

// GetNumeric retrieves the numeric value from the JSON node
func (n *AJSONValue) GetNumeric() mo.Result[float64] {
	num, err := n.node.GetNumeric()
	if err != nil {
		return mo.Err[float64](err)
	}

	return mo.Ok(num)
}

// GetInt retrieves the integer value from the JSON node
func (n *AJSONValue) GetInt() mo.Result[int] {
	num, err := n.node.GetNumeric()
	if err != nil {
		return mo.Err[int](err)
	}

	intNum := int(num)

	if num-float64(intNum) != 0 {
		return mo.Err[int](fmt.Errorf("value is not an integer"))
	}

	return mo.Ok(intNum)
}

// GetString retrieves the string value from the JSON node
func (n *AJSONValue) GetString() mo.Result[string] {
	if !n.IsString() {
		return mo.Err[string](fmt.Errorf("current value is not a string"))
	}

	str, err := n.node.GetString()
	if err != nil {
		return mo.Err[string](err)
	}

	return mo.Ok(str)
}

// SetKey sets a key-value pair in the JSON object
func (n *AJSONValue) SetKey(key string, value Value) error {
	vnode, ok := value.(*AJSONValue)
	if !ok {
		return fmt.Errorf("value is not an AJSONValue")
	}
	childNode, err := n.node.GetKey(key)
	if err != nil {
		err := n.node.AppendObject(key, vnode.node)
		return err
	}

	switch vnode.node.Type() {
	case ajson.Numeric:
		return childNode.SetNumeric(vnode.node.MustNumeric())
	case ajson.String:
		return childNode.SetString(vnode.node.MustString())
	case ajson.Bool:
		return childNode.SetBool(vnode.node.MustBool())
	case ajson.Null:
		return childNode.SetNull()
	case ajson.Object:
		return childNode.SetObject(vnode.node.MustObject())
	case ajson.Array:
		return childNode.SetArray(vnode.node.MustArray())
	default:
		return NewError(BadPath).Append("attempt to set unsupported type in JSON object")
	}
}

// DeleteKey deletes a key from the JSON object
func (n *AJSONValue) DeleteKey(key string) error {
	if !n.IsObject() {
		return fmt.Errorf("current value is not an object")
	}

	err := n.node.DeleteKey(key)
	if err != nil {
		return err
	}

	return nil
}

// HasKey checks if the JSON object has a specific key
func (n *AJSONValue) HasKey(key string) bool {
	return n.node.HasKey(key)
}

// Size returns the number of properties in the JSON object
func (n *AJSONValue) Size() int {
	return n.node.Size()
}

// Equals checks if two JSON nodes are equal
func (n *AJSONValue) Equals(otherNode Value) bool {
	other, _ := otherNode.(*AJSONValue)
	if n.node.Type() != other.node.Type() {
		return false
	}
	eq, err := n.node.Eq(other.node)
	if err != nil {
		return false
	}

	return eq
}

// RawMessage returns the raw JSON message of the node
func (n *AJSONValue) RawMessage() json.RawMessage {
	val, err := n.node.Unpack()
	if err != nil {
		return nil
	}

	bytes, _ := json.Marshal(val)
	return bytes
}

// Format 实现 fmt.Formatter 接口，用于格式化输出
func (n *AJSONValue) Format(st fmt.State, _ rune) {
	_, _ = fmt.Fprint(st, string(n.RawMessage()))
}
