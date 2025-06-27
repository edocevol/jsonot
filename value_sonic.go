package jsonot

import (
	"encoding/json"
	"fmt"

	"github.com/bytedance/sonic/ast"
	"github.com/samber/mo"
)

var _ Value = (*SonicValue)(nil)

// SonicValue 定义了 JSON Object 的节点
type SonicValue struct {
	node *ast.Node
}

// NewSonicValue 创建一个新的 SonicValue
func NewSonicValue(node *ast.Node) *SonicValue {
	return &SonicValue{node: node}
}

// Format 实现 fmt.Formatter 接口，用于格式化输出
func (n *SonicValue) Format(st fmt.State, verb rune) {
	jsonStr, _ := n.node.Raw()
	_, _ = fmt.Fprint(st, jsonStr)
}

// RawMessage 返回节点的原始字节数据
func (n *SonicValue) RawMessage() json.RawMessage {
	data, _ := n.node.Raw()
	return json.RawMessage(data)
}

// Unmarshal 实现 JSON 的解码
func (n *SonicValue) Unmarshal(data []byte) error {
	if n.node == nil {
		n.node = &ast.Node{}
	}
	err := n.node.UnmarshalJSON(data)
	if err != nil {
		return NewError(UnexpectedError).Wrap(err)
	}

	return nil
}

// UpdateArray 从传入的 Value 数组更新当前 Value
func (n *SonicValue) UpdateArray(values []Value) error {
	var nodes []ast.Node
	for k := range values {
		sn, ok := values[k].(*SonicValue)
		if !ok {
			continue
		}
		nodes = append(nodes, *sn.node)
	}

	node := ast.NewArray(nodes)
	n.node = &node

	return nil
}

// UpdateObject 从传入的 Value 更新当前 Value
func (n *SonicValue) UpdateObject(value Value) error {
	sn, ok := value.(*SonicValue)
	if !ok {
		return fmt.Errorf("expected SonicValue, got: %T", value)
	}

	if sn.node == nil {
		return fmt.Errorf("node is nil")
	}

	n.node = sn.node
	return nil
}

// IsBool 返回节点是否为布尔类型
func (n *SonicValue) IsBool() bool {
	nodeType := n.node.TypeSafe()
	if nodeType == ast.V_TRUE || nodeType == ast.V_FALSE {
		return true
	}

	_, err := n.node.StrictBool()
	return err == nil
}

// IsNull 返回节点是否为 null 类型
func (n *SonicValue) IsNull() bool {
	return n.node.TypeSafe() == ast.V_NULL
}

// IsNumber 返回节点是否为数字类型
func (n *SonicValue) IsNumber() bool {
	if n.node.TypeSafe() == ast.V_NUMBER {
		return true
	}

	_, err := n.node.Float64()
	return err == nil
}

// IsString 返回节点是否为字符串类型
func (n *SonicValue) IsString() bool {
	if n.node.TypeSafe() == ast.V_STRING {
		return true
	}

	str, err := n.node.StrictString()
	return err == nil && str != ""
}

// IsArray 返回节点是否为数组类型
func (n *SonicValue) IsArray() bool {
	if n.node.TypeSafe() == ast.V_ARRAY {
		return true
	}

	_, err := n.node.Array()
	return err == nil
}

// IsObject 返回节点是否为对象类型
func (n *SonicValue) IsObject() bool {
	if n.node.TypeSafe() == ast.V_OBJECT {
		return true
	}

	_, err := n.node.Map()
	return err == nil
}

// IsNumeric 返回节点是否为数字类型
func (n *SonicValue) IsNumeric() bool {
	return n.node.TypeSafe() == ast.V_NUMBER
}

// IsInt 返回节点是否为整数类型
func (n *SonicValue) IsInt() bool {
	val, err := n.node.Float64()
	return err == nil && val == float64(int(val))
}

// IsFloat 返回节点是否为浮点数类型
func (n *SonicValue) IsFloat() bool {
	_, err := n.node.StrictFloat64()
	return err == nil
}

// Type 返回节点的类型
func (n *SonicValue) Type() ValueType {
	ts := n.node.TypeSafe()
	switch ts {
	case ast.V_NULL:
		return Null
	case ast.V_ERROR:
		return Null
	case ast.V_TRUE:
		return Bool
	case ast.V_FALSE:
		return Bool
	case ast.V_ARRAY:
		return Array
	case ast.V_OBJECT:
		return Object
	case ast.V_NUMBER:
		return Numeric
	case ast.V_STRING:
		return String
	case ast.V_ANY:
		return Any
	}
	return ValueType(ts) // 默认返回 Null 类型
}

// PackAny 将 SonicValue 解包为 ast.NewValue
func (n *SonicValue) PackAny() {
	if n.node.TypeSafe() != ast.V_ANY {
		return
	}

	// 将 any 类型的节点重新解析为具体类型
	data, _ := n.node.Raw()
	node, err := ast.NewParser(data).Parse()
	if err != 0 {
		return
	}
	n.node = &node
}

// GetKey 返回节点的键名，如果节点是对象类型且存在键名
func (n *SonicValue) GetKey(key string) mo.Option[Value] {
	n.PackAny()
	node := n.node.Get(key)
	if IsErrorSonicValue(node) {
		return mo.None[Value]()
	}
	return mo.Some[Value](NewSonicValue(node))
}

// GetByPath 返回节点的键名，如果节点是对象类型且存在键名
func (n *SonicValue) GetByPath(path ...any) mo.Option[Value] {
	node := n.node.GetByPath(path)
	if IsErrorSonicValue(node) {
		return mo.None[Value]()
	}
	newNode := NewSonicValue(node)
	newNode.PackAny()
	return mo.Some[Value](newNode)
}

// GetIndex 返回节点的索引值，如果节点是数组类型且存在索引
func (n *SonicValue) GetIndex(index int) mo.Option[Value] {
	n.PackAny()
	if !n.IsArray() {
		return mo.None[Value]()
	}
	node := n.node.Index(index)
	if IsErrorSonicValue(node) {
		return mo.None[Value]()
	}
	return mo.Some[Value](NewSonicValue(node))
}

// GetStringKey 返回节点的字符串键名，如果节点是对象类型且存在字符串键名
func (n *SonicValue) GetStringKey(key string) mo.Option[string] {
	n.PackAny()
	if !n.IsObject() {
		return mo.None[string]()
	}

	node := n.node.Get(key)
	str, err := node.StrictString()
	if err != nil {
		return mo.None[string]()
	}

	return mo.Some[string](str)
}

// GetIntKey 返回节点的整数键名，如果节点是对象类型且存在整数键名
func (n *SonicValue) GetIntKey(key string) mo.Option[int] {
	n.PackAny()
	if !n.IsObject() {
		return mo.None[int]()
	}

	node := n.node.Get(key)
	num, err := node.StrictInt64()
	if err != nil {
		return mo.None[int]()
	}

	return mo.Some[int](int(num))
}

// GetArray 返回节点的数组，如果节点是数组类型
func (n *SonicValue) GetArray() mo.Result[[]Value] {
	nodes, err := n.node.ArrayUseNode()
	if err != nil {
		return mo.Err[[]Value](err)
	}

	result := make([]Value, len(nodes))
	for i, node := range nodes {
		result[i] = NewSonicValue(&node)
	}

	return mo.Ok[[]Value](result)
}

// GetMap 返回节点的映射，如果节点是对象类型
func (n *SonicValue) GetMap() mo.Result[map[string]Value] {
	properties, err := n.node.MapUseNode()
	if err != nil {
		return mo.Err[map[string]Value](err)
	}

	result := make(map[string]Value, len(properties))
	for key, value := range properties {
		result[key] = NewSonicValue(&value)
	}

	return mo.Ok[map[string]Value](result)
}

// GetNumeric 返回节点的数字值，如果节点是数字类型
func (n *SonicValue) GetNumeric() mo.Result[float64] {
	num, err := n.node.Float64()
	if err != nil {
		return mo.Err[float64](err)
	}

	return mo.Ok[float64](num)
}

// GetInt 返回节点的整数值，如果节点是数字类型
func (n *SonicValue) GetInt() mo.Result[int] {
	num, err := n.node.Int64()
	if err != nil {
		return mo.Err[int](err)
	}

	return mo.Ok[int](int(num))
}

// GetString 返回节点的字符串值，如果节点是字符串类型
func (n *SonicValue) GetString() mo.Result[string] {
	if !n.IsString() {
		return mo.Err[string](fmt.Errorf("current value is not a string"))
	}

	str, err := n.node.String()
	if err != nil {
		return mo.Err[string](err)
	}

	return mo.Ok[string](str)
}

// SetKey 设置对象中的键名和值
func (n *SonicValue) SetKey(key string, value Value) error {
	if !n.IsObject() {
		return fmt.Errorf("current value is not an object")
	}

	node, _ := value.(*SonicValue) // always should be a SonicValue
	_, err := n.node.Set(key, *node.node)
	return err
}

// DeleteKey 从对象中删除指定的键名
func (n *SonicValue) DeleteKey(key string) error {
	if !n.IsObject() {
		return nil // 或者返回一个错误
	}
	_, _ = n.node.Unset(key) // 不存在时忽略错误
	return nil
}

// HasKey 检查对象中是否存在指定的键名
func (n *SonicValue) HasKey(key string) bool {
	if !n.IsObject() {
		return false
	}
	node := n.node.Get(key)
	return !IsErrorSonicValue(node) && node.TypeSafe() != ast.V_NULL
}

// Size 返回对象的属性数量
func (n *SonicValue) Size() int {
	if n.IsObject() {
		properties, err := n.node.Map()
		if err != nil {
			return 0 // 如果发生错误，返回 0
		}
		return len(properties)
	}

	if n.IsArray() {
		nodes, err := n.node.Array()
		if err != nil {
			return 0 // 如果发生错误，返回 0
		}
		return len(nodes)
	}

	return 0
}

// Equals 检查两个节点是否相等
func (n *SonicValue) Equals(other Value) bool {
	if other == nil {
		return false
	}
	otherNode, ok := other.(*SonicValue)
	if !ok {
		return false
	}

	currentStr, _ := n.node.Raw()
	otherStr, _ := otherNode.node.Raw()

	return currentStr == otherStr
}

// IsErrorSonicValue 检查节点是否为 SonicValue 错误类型
func IsErrorSonicValue(node *ast.Node) bool {
	if node == nil {
		return true
	}
	return node.TypeSafe() == ast.V_ERROR
}
