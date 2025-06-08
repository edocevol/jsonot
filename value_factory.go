package jsonot

import (
	"encoding/json"

	"github.com/bytedance/sonic/ast"
	"github.com/spyzhov/ajson"
)

// UseSonic indicates whether to use the Sonic JSON library for JSON operations.
var useSonic = true

// UseSonicJSON sets the flag to use the Sonic JSON library.
func UseSonicJSON() {
	useSonic = true
}

// UnmarshalValue 创建一个新的 JSON Object 节点
func UnmarshalValue(data []byte) (Value, error) {
	if useSonic {
		node, _ := ast.NewParser(string(data)).Parse()
		return NewSonicNode(&node), nil
	}

	node, err := ajson.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	return NewAJSONValue(node), nil
}

// ValueFromPrimitive 创建一个新的 JSON Object 节点
func ValueFromPrimitive[T string | int | int64 | float64](data T) Value {
	if useSonic {
		node := ast.NewAny(data)
		return NewSonicNode(&node)
	}

	var node *ajson.Node
	switch v := any(data).(type) {
	case string:
		node = ajson.StringNode("", v)
	case int:
		node = ajson.NumericNode("", float64(v))
	case int64:
		node = ajson.NumericNode("", float64(v))
	case float64:
		node = ajson.NumericNode("", v)
	}

	return NewAJSONValue(node)
}

// ValueFromAny 创建一个新的 JSON Object 节点
func ValueFromAny(data any) Value {
	if useSonic {
		var node ast.Node
		bytes, _ := json.Marshal(data)
		_ = node.UnmarshalJSON(bytes)
		return NewSonicNode(&node)
	}

	bytes, _ := json.Marshal(data)
	node, _ := ajson.Unmarshal(bytes)
	return NewAJSONValue(node)
}

// ValueFromArray 创建一个新的 JSON Array 节点
func ValueFromArray(arr []Value) Value {
	if useSonic {
		var nodes []ast.Node
		for _, item := range arr {
			if sonicNode, ok := item.(*SonicNode); ok {
				nodes = append(nodes, *sonicNode.node)
			}
		}
		node := ast.NewArray(nodes)
		return NewSonicNode(&node)
	}

	var nodes []*ajson.Node
	for _, item := range arr {
		if aNode, ok := item.(*AJSONValue); ok {
			nodes = append(nodes, aNode.node.Clone())
		}
	}

	node := ajson.ArrayNode("", nodes)
	nodeVal, _ := node.Value()
	log.Debugf("ValueFromArray: %v\n", nodeVal)
	return NewAJSONValue(node)
}
