package jsonot

import (
	"encoding/json"
	"fmt"

	"github.com/bytedance/sonic/ast"
	"github.com/samber/mo"
)

var _ Value = (*SonicNode)(nil)

// SonicNode 定义了 JSON Object 的节点
type SonicNode struct {
	node *ast.Node
}

// NewSonicNode 创建一个新的 SonicNode
func NewSonicNode(node *ast.Node) *SonicNode {
	return &SonicNode{node: node}
}

// Format 实现 fmt.Formatter 接口，用于格式化输出
func (n *SonicNode) Format(st fmt.State, verb rune) {
	jsonStr, _ := n.node.Raw()
	_, _ = fmt.Fprint(st, jsonStr)
}

// RawMessage 返回节点的原始字节数据
func (n *SonicNode) RawMessage() json.RawMessage {
	data, _ := n.node.Raw()
	return json.RawMessage(data)
}

// Unmarshal 实现 JSON 的解码
func (n *SonicNode) Unmarshal(data []byte) error {
	if n.node == nil {
		n.node = &ast.Node{}
	}
	return n.node.UnmarshalJSON(data)
}

// IsBool 返回节点是否为布尔类型
func (n *SonicNode) IsBool() bool {
	nodeType := n.node.TypeSafe()
	if nodeType == ast.V_TRUE || nodeType == ast.V_FALSE {
		return true
	}

	_, err := n.node.StrictBool()
	return err == nil
}

// IsNull 返回节点是否为 null 类型
func (n *SonicNode) IsNull() bool {
	return n.node.TypeSafe() == ast.V_NULL
}

// IsNumber 返回节点是否为数字类型
func (n *SonicNode) IsNumber() bool {
	if n.node.TypeSafe() == ast.V_NUMBER {
		return true
	}

	_, err := n.node.Float64()
	return err == nil
}

// IsString 返回节点是否为字符串类型
func (n *SonicNode) IsString() bool {
	if n.node.TypeSafe() == ast.V_STRING {
		return true
	}

	str, err := n.node.StrictString()
	return err == nil && str != ""
}

// IsArray 返回节点是否为数组类型
func (n *SonicNode) IsArray() bool {
	if n.node.TypeSafe() == ast.V_ARRAY {
		return true
	}

	_, err := n.node.Array()
	return err == nil
}

// IsObject 返回节点是否为对象类型
func (n *SonicNode) IsObject() bool {
	if n.node.TypeSafe() == ast.V_OBJECT {
		return true
	}

	_, err := n.node.Map()
	return err == nil
}

// IsNumeric 返回节点是否为数字类型
func (n *SonicNode) IsNumeric() bool {
	return n.node.TypeSafe() == ast.V_NUMBER
}

// IsInt 返回节点是否为整数类型
func (n *SonicNode) IsInt() bool {
	_, err := n.node.Int64()
	return err == nil
}

// IsFloat 返回节点是否为浮点数类型
func (n *SonicNode) IsFloat() bool {
	_, err := n.node.StrictFloat64()
	return err == nil
}

// Type 返回节点的类型
func (n *SonicNode) Type() NodeType {
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
	return NodeType(ts) // 默认返回 Null 类型
}

// PackAny 将 SonicNode 解包为 ast.Value
func (n *SonicNode) PackAny() {
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
func (n *SonicNode) GetKey(key string) mo.Option[Value] {
	n.PackAny()
	node := n.node.Get(key)
	if IsErrorSonicNode(node) {
		return mo.None[Value]()
	}
	return mo.Some[Value](NewSonicNode(node))
}

// GetByPath 返回节点的键名，如果节点是对象类型且存在键名
func (n *SonicNode) GetByPath(path ...any) mo.Option[Value] {
	node := n.node.GetByPath(path)
	if IsErrorSonicNode(node) {
		return mo.None[Value]()
	}
	newNode := NewSonicNode(node)
	newNode.PackAny()
	return mo.Some[Value](newNode)
}

// GetIndex 返回节点的索引值，如果节点是数组类型且存在索引
func (n *SonicNode) GetIndex(index int) mo.Option[Value] {
	n.PackAny()
	if !n.IsArray() {
		return mo.None[Value]()
	}
	node := n.node.Index(index)
	if IsErrorSonicNode(node) {
		return mo.None[Value]()
	}
	return mo.Some[Value](NewSonicNode(node))
}

// GetStringKey 返回节点的字符串键名，如果节点是对象类型且存在字符串键名
func (n *SonicNode) GetStringKey(key string) mo.Option[string] {
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
func (n *SonicNode) GetIntKey(key string) mo.Option[int] {
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
func (n *SonicNode) GetArray() mo.Result[[]Value] {
	nodes, err := n.node.Array()
	if err != nil {
		return mo.Err[[]Value](err)
	}

	result := make([]Value, len(nodes))
	for i, node := range nodes {
		astNode := ast.NewAny(node)
		result[i] = NewSonicNode(&astNode)
	}

	return mo.Ok[[]Value](result)
}

// GetMap 返回节点的映射，如果节点是对象类型
func (n *SonicNode) GetMap() mo.Result[map[string]Value] {
	properties, err := n.node.Map()
	if err != nil {
		return mo.Err[map[string]Value](err)
	}

	result := make(map[string]Value, len(properties))
	for key, value := range properties {
		astNode := ast.NewAny(value)
		result[key] = NewSonicNode(&astNode)
	}

	return mo.Ok[map[string]Value](result)
}

// GetNumeric 返回节点的数字值，如果节点是数字类型
func (n *SonicNode) GetNumeric() mo.Result[float64] {
	num, err := n.node.Float64()
	if err != nil {
		return mo.Err[float64](err)
	}

	return mo.Ok[float64](num)
}

// GetInt 返回节点的整数值，如果节点是数字类型
func (n *SonicNode) GetInt() mo.Result[int] {
	num, err := n.node.Int64()
	if err != nil {
		return mo.Err[int](err)
	}

	return mo.Ok[int](int(num))
}

// GetString 返回节点的字符串值，如果节点是字符串类型
func (n *SonicNode) GetString() mo.Result[string] {
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
func (n *SonicNode) SetKey(key string, value Value) error {
	if !n.IsObject() {
		return fmt.Errorf("current value is not an object")
	}

	node, _ := value.(*SonicNode) // always should be a SonicNode
	_, err := n.node.Set(key, *node.node)
	return err
}

// DeleteKey 从对象中删除指定的键名
func (n *SonicNode) DeleteKey(key string) error {
	if !n.IsObject() {
		return nil // 或者返回一个错误
	}
	_, _ = n.node.Unset(key) // 不存在时忽略错误
	return nil
}

// HasKey 检查对象中是否存在指定的键名
func (n *SonicNode) HasKey(key string) bool {
	if !n.IsObject() {
		return false
	}
	node := n.node.Get(key)
	return !IsErrorSonicNode(node) && node.TypeSafe() != ast.V_NULL
}

// Size 返回对象的属性数量
func (n *SonicNode) Size() int {
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
func (n *SonicNode) Equals(other Value) bool {
	if other == nil {
		return false
	}
	otherNode, ok := other.(*SonicNode)
	if !ok {
		return false
	}

	currentStr, _ := n.node.Raw()
	otherStr, _ := otherNode.node.Raw()

	return currentStr == otherStr
}

// IsErrorSonicNode 检查节点是否为 SonicNode 错误类型
func IsErrorSonicNode(node *ast.Node) bool {
	if node == nil {
		return true
	}
	return node.TypeSafe() == ast.V_ERROR
}
