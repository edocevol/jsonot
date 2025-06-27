package jsonot

import (
	"context"
	"fmt"

	"github.com/samber/mo"
)

// UseSonic 用于切换到 Sonic 实现
func UseSonic() {
	defaultValueFactory = NewSonicValueFactory()
}

// UseAJSON 切换到 AJSON 实现
func UseAJSON() {
	defaultValueFactory = NewAJSONValueFactory()
}

// NewJSONOperationTransformer 创建一个新的 JSONOperationTransformer
func NewJSONOperationTransformer() *JSONOperationTransformer {
	ot := new(JSONOperationTransformer)
	ot.transformer = NewTransformer()
	ot.functions = NewSubTypeFunctionsHolder()
	ot.operationFaction = NewOperationFactory(ot.functions)
	return ot
}

// OperationTransformer 是一个接口，用于转换 JSON 操作
type OperationTransformer interface {
	// OperationFromValueArray 创建一个操作
	OperationFromValueArray(values []Value) mo.Result[*Operation]
	// OperationComponentsFromValue 创建一个操作组件
	OperationComponentsFromValue(node Value) mo.Result[[]*OperationComponent]
	// Apply 将操作应用到给定的 JSON 节点上
	Apply(ctx context.Context, value Value, operation *Operation) mo.Result[Value]
	// Applies 将多个操作应用到给定的 JSON 节点上
	Applies(ctx context.Context, value Value, operations []*Operation) mo.Result[Value]
	// Transform 对操作进行转换
	Transform(ctx context.Context, left, right *Operation) (leftN, rightN *Operation, err error)
}

var _ OperationTransformer = (*JSONOperationTransformer)(nil)

// JSONOperationTransformer 是一个实现了 OperationTransformer 接口的结构体
type JSONOperationTransformer struct {
	functions        SubTypeFunctionsHolder
	transformer      *Transformer
	operationFaction *OperationFactory
}

// RegisterSubType 注册子类型函数
func (ot *JSONOperationTransformer) RegisterSubType(subType string, fn SubTypeFunctions) {
	ot.functions.Register(SubTypeAction(subType), fn)
}

// UnregisterSubType 注销子类型函数
func (ot *JSONOperationTransformer) UnregisterSubType(subType string) {
	ot.functions.Unregister(SubTypeAction(subType))
}

// OperationFactory 返回操作工厂
func (ot *JSONOperationTransformer) OperationFactory() *OperationFactory {
	return ot.operationFaction
}

// OperationComponentsFromValue 创建一个操作组件
func (ot *JSONOperationTransformer) OperationComponentsFromValue(
	node Value,
) mo.Result[[]*OperationComponent] {
	if node.IsArray() {
		arr := node.GetArray()
		if arr.IsError() {
			return mo.Err[[]*OperationComponent](arr.Error())
		}
		var result []*OperationComponent
		for _, item := range arr.MustGet() {
			v := ot.OperationComponentsFromValue(item)
			if v.IsOk() {
				result = append(result, v.MustGet()...)
			} else {
				return mo.Err[[]*OperationComponent](v.Error())
			}
		}
		return mo.Ok(result)
	}

	if node.IsObject() {
		result := ot.operationFaction.OperationComponentFromValue(node)
		if result.IsError() {
			return mo.Err[[]*OperationComponent](result.Error())
		}
		return mo.Ok([]*OperationComponent{result.MustGet()})
	}

	return mo.Err[[]*OperationComponent](fmt.Errorf("unsupported node type: %d", node.Type()))
}

// OperationComponentFromValue 创建一个操作组件
func (ot *JSONOperationTransformer) OperationComponentFromValue(value Value) mo.Result[*OperationComponent] {
	if !value.IsObject() {
		return mo.Err[*OperationComponent](fmt.Errorf("expected object node, got: %s", value.Type()))
	}
	return ot.operationFaction.OperationComponentFromValue(value)
}

// OperationFromValueArray 创建一个操作
func (ot *JSONOperationTransformer) OperationFromValueArray(values []Value) mo.Result[*Operation] {
	var components []*OperationComponent
	for k := range values {
		v := ot.OperationComponentsFromValue(values[k])
		if v.IsOk() {
			components = append(components, v.MustGet()...)
		} else {
			return mo.Err[*Operation](v.Error())
		}
	}

	operation := NewOperation(components)
	return mo.Ok(operation)
}

// Apply 将操作应用到给定的 JSON 节点上
func (ot *JSONOperationTransformer) Apply(
	_ context.Context, value Value, operations *Operation,
) mo.Result[Value] {
	if operations.Len() == 0 {
		return mo.Ok(value)
	}

	components := operations.Array()
	for _, op := range components {
		if err := op.Validation(); err != nil {
			return mo.Err[Value](err)
		}
	}

	value.PackAny()
	var err error
	for k := range components {
		err = ApplyToValue(value, components[k].Path, components[k].Operator)
		if err != nil {
			return mo.Err[Value](err)
		}
	}

	return mo.Ok(value)
}

// Applies 将多个操作应用到给定的 JSON 节点上
func (ot *JSONOperationTransformer) Applies(
	ctx context.Context, value Value, operations []*Operation,
) mo.Result[Value] {
	if len(operations) == 0 {
		return mo.Ok(value)
	}

	for _, op := range operations {
		if op.Len() == 0 {
			continue
		}

		result := ot.Apply(ctx, value, op)
		if result.IsError() {
			return result
		}
		value = result.MustGet()
	}

	return mo.Ok(value)
}

// Transform 将操作转换为另一种形式
func (ot *JSONOperationTransformer) Transform(
	_ context.Context, left, right *Operation,
) (newLeft, newRight *Operation, err error) {
	newLeftResult, newRightResult, err := ot.transformer.Transform(left, right)
	if err != nil {
		return nil, nil, err
	}

	if newLeftResult.IsOk() && newRightResult.IsOk() {
		return newLeftResult.MustGet(), newRightResult.MustGet(), nil
	}

	if newLeftResult.IsOk() {
		return newLeftResult.MustGet(), NewOperation([]*OperationComponent{}), nil
	}

	if newRightResult.IsOk() {
		return NewOperation([]*OperationComponent{}), newRightResult.MustGet(), nil
	}

	err = fmt.Errorf("transform failed left: %w, right: %w", newLeftResult.Error(), newRightResult.Error())
	return nil, nil, err
}
