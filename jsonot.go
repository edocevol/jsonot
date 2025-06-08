package jsonot

import (
	"context"
	"fmt"

	"github.com/samber/mo"
)

// OperationTransformer 是一个接口，用于转换 JSON 操作
type OperationTransformer interface {
	// Apply 将操作应用到给定的 JSON 节点上
	Apply(ctx context.Context, value Value, operation *Operation) mo.Result[Value]
	// Transform 将操作转换为另一种形式
	Transform(
		ctx context.Context, baseOperations, newOperations *Operation,
	) (left *Operation, right *Operation, err error)
	// Compose 将多个操作组合成一个操作
	Compose(ctx context.Context, operations *Operation) mo.Result[*Operation]
	// Invert 将操作反转
	Invert(ctx context.Context, operations *Operation) mo.Result[*Operation]
}

// JSONOperationTransformer 是一个实现了 OperationTransformer 接口的结构体
type JSONOperationTransformer struct {
	functions        SubTypeFunctionsHolder
	transformer      *Transformer
	operationFaction *OperationFactory
}

// NewJSONOperationTransformer 创建一个新的 JSONOperationTransformer
func NewJSONOperationTransformer() *JSONOperationTransformer {
	ot := new(JSONOperationTransformer)
	ot.transformer = NewTransformer()
	ot.functions = NewSubTypeFunctionsHolder()
	ot.functions.Register(ActionSubTypeNumberAdd, NewNumberAddSubType())
	ot.functions.Register(ActionSubTypeText, NewTextSubType())
	ot.operationFaction = NewOperationFactory(ot.functions)

	return ot
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

// OperationComponentsFromNode 创建一个操作组件
func (ot *JSONOperationTransformer) OperationComponentsFromNode(
	node Value,
) mo.Result[[]*OperationComponent] {
	if node.IsArray() {
		arr := node.GetArray()
		if arr.IsError() {
			return mo.Err[[]*OperationComponent](arr.Error())
		}
		var result []*OperationComponent
		for _, item := range arr.MustGet() {
			v := ot.OperationComponentsFromNode(item)
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

// Apply 将操作应用到给定的 JSON 节点上
func (ot *JSONOperationTransformer) Apply(
	ctx context.Context, value Value, operations *Operation,
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

	var err error
	for _, op := range components {
		err = ApplyToValue(value, op.Path, op.Operator)
		if err != nil {
			return mo.Err[Value](err)
		}
	}

	return mo.Ok(value)
}

// Transform 将操作转换为另一种形式
func (ot *JSONOperationTransformer) Transform(
	ctx context.Context, newOperation, baseOperation *Operation,
) (*Operation, *Operation, error) {
	left, right, err := ot.transformer.Transform(newOperation, baseOperation)
	if err != nil {
		return nil, nil, err
	}

	if left.IsOk() && right.IsOk() {
		return left.MustGet(), right.MustGet(), nil
	}

	if left.IsOk() {
		return left.MustGet(), NewOperation([]*OperationComponent{}), nil
	}

	if right.IsOk() {
		return NewOperation([]*OperationComponent{}), right.MustGet(), nil
	}

	return nil, nil, fmt.Errorf("transform failed: %w", left.Error())
}

// Invert 将操作反转
func (ot *JSONOperationTransformer) Invert(
	ctx context.Context, operations *Operation,
) mo.Result[*Operation] {
	if operations.Len() == 0 {
		return mo.Ok(NewOperation([]*OperationComponent{}))
	}

	invertedComponents := make([]*OperationComponent, 0, operations.Len())
	for _, op := range operations.Array() {
		inverted := op.Invert()
		if inverted.IsError() {
			return mo.Err[*Operation](inverted.Error())
		}
		invertedComponents = append(invertedComponents, inverted.MustGet())
	}

	return mo.Ok(NewOperation(invertedComponents))
}
