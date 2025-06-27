package jsonot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapToOperatorForListMove(t *testing.T) {
	factory := NewOperationFactory(nil)

	// 构造有效的 JSON 节点
	jsonStr := `{"lm": 3, "p":["some", "path"]}`
	value, err := UnmarshalValue([]byte(jsonStr))
	assert.Nil(t, err)
	result := factory.MapToOperator(value)
	assert.True(t, result.IsOk())
	assert.IsType(t, &ListMove{}, result.MustGet())

	// 构造无效的 JSON 节点
	invalidJSONStr := `{"lm": "invalid", "p":["some", "path"]}`
	invalidValue, _ := UnmarshalValue([]byte(invalidJSONStr))

	result = factory.MapToOperator(invalidValue)
	assert.True(t, result.IsError())
}

func TestMapToOperatorForListInsert(t *testing.T) {
	factory := NewOperationFactory(nil)

	// 构造有效的 JSON 节点
	jsonStr := `{"li": 1, "p":["some", "path"]}`
	value, _ := UnmarshalValue([]byte(jsonStr))

	result := factory.MapToOperatorForListInsert(value)
	assert.True(t, result.IsOk())
	assert.IsType(t, &ListInsert{}, result.MustGet())
}

func TestMapToOperatorForListDelete(t *testing.T) {
	factory := NewOperationFactory(nil)

	// 构造有效的 JSON 节点
	jsonStr := `{"ld": 1, "p":["some", "path"]}`
	value, _ := UnmarshalValue([]byte(jsonStr))

	result := factory.MapToOperatorForListDelete(value)
	assert.True(t, result.IsOk())
	assert.IsType(t, &ListDelete{}, result.MustGet())
}

func TestMapToOperatorForObjectInsert(t *testing.T) {
	factory := NewOperationFactory(nil)

	// 构造有效的 JSON 节点
	jsonStr := `{"oi": "value", "p":["some", "path"]}`
	value, _ := UnmarshalValue([]byte(jsonStr))

	result := factory.MapToOperatorForObjectInsert(value)
	assert.True(t, result.IsOk())
	assert.IsType(t, &ObjectInsert{}, result.MustGet())
}

func TestMapToOperatorForObjectDelete(t *testing.T) {
	factory := NewOperationFactory(nil)

	// 构造有效的 JSON 节点
	jsonStr := `{"od": "value", "p":["some", "path"]}`
	value, _ := UnmarshalValue([]byte(jsonStr))

	result := factory.MapToOperatorForObjectDelete(value)
	assert.True(t, result.IsOk())
	assert.IsType(t, &ObjectDelete{}, result.MustGet())
}
