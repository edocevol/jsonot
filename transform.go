package jsonot

import (
	"github.com/samber/mo"
)

// TransformSide 表示转换的方向
type TransformSide int

const (
	// TransformSideLeft 表示转换的方向是从左边
	TransformSideLeft TransformSide = iota
	// TransformSideRight 表示转换的方向是从右边
	TransformSideRight
)

// IsEquivalentToNoop 判断一个操作是否等价于 Noop
func IsEquivalentToNoop(op *OperationComponent) bool {
	switch v1 := op.Operator.(type) {
	case *Noop:
		return true
	case *SubTypeOperator:
		return false
	case *ListInsert, *ListDelete, *ObjectInsert, *ObjectDelete:
		return false
	case *ListReplace:
		// 如果新值等于旧值，则等价于 Noop
		return v1.NewValue.Equals(v1.OldValue)
	case *ObjectReplace:
		// 如果新值等于旧值，则等价于 Noop
		return v1.NewValue.Equals(v1.OldValue)
	case *ListMove:
		// 如果操作的路径的最后一个元素是新值的索引，则等价于 Noop
		lastPath := op.Path.Last()
		if lastPath.IsPresent() && lastPath.MustGet().Index == v1.NewIndex {
			return true
		}
	}
	return false
}

// IsSameOperand 判断两个操作是否操作相同的操作数
func IsSameOperand(opA, opB *OperationComponent) bool {
	if _, ok := opA.Operator.(*SubTypeOperator); ok {
		return false
	}
	if _, ok := opB.Operator.(*SubTypeOperator); ok {
		return false
	}
	return opA.Path.Len() == opB.Path.Len()
}

// Transformer 表示一个操作转换器
type Transformer struct {
}

// NewTransformer 创建一个操作转换器
func NewTransformer() *Transformer {
	return &Transformer{}
}

// Transform 对一个操作进行转换
func (t *Transformer) Transform(
	operation, baseOperation *Operation,
) (leftTransformed, rightTransformed mo.Result[*Operation], err error) {
	if baseOperation.IsEmpty() {
		return mo.Ok(operation), mo.Ok(EmptyOperation()), nil
	}

	if err := operation.Validation(); err != nil {
		return mo.Err[*Operation](err), mo.Err[*Operation](err), err
	}
	if err := baseOperation.Validation(); err != nil {
		return mo.Err[*Operation](err), mo.Err[*Operation](err), err
	}

	if operation.Len() == 1 && baseOperation.Len() == 1 {
		newOp, _ := operation.Operations.Front().Value.(*OperationComponent)
		baseOp, _ := baseOperation.Operations.Front().Value.(*OperationComponent)

		var left, right mo.Result[[]*OperationComponent]
		if left, err = t.TransformComponent(newOp.Clone(), baseOp.Clone(), TransformSideLeft); err != nil {
			return mo.Err[*Operation](err), mo.Err[*Operation](err), err
		}

		if right, err = t.TransformComponent(baseOp.Clone(), newOp.Clone(), TransformSideRight); err != nil {
			return mo.Err[*Operation](err), mo.Err[*Operation](err), err
		}

		return mo.Ok(NewOperation(left.MustGet())), mo.Ok(NewOperation(right.MustGet())), nil
	}

	// 如果是多个操作，则需要转换整个操作矩阵
	return t.TransformMatrix(operation, baseOperation)
}

// TransformMatrix 对一个操作矩阵进行转换
//
// 要求左右的操作都是非空的
func (t *Transformer) TransformMatrix(
	operation, baseOperation *Operation,
) (lop, rop mo.Result[*Operation], err error) {
	if operation.IsEmpty() || baseOperation.IsEmpty() {
		return mo.Ok(operation), mo.Ok(baseOperation), nil
	}

	var outB []*OperationComponent
	ops := operation // 输入的
	for baseOp := baseOperation.Operations.Front(); baseOp != nil; baseOp = baseOp.Next() {
		baseOp, _ := baseOp.Value.(*OperationComponent)
		a, b, err := t.TransformMulti(ops, baseOp)
		if err != nil {
			return mo.Err[*Operation](err), mo.Err[*Operation](err), err
		}
		ops = a.MustGet()
		if b.IsOk() {
			outB = append(outB, b.MustGet())
		}
	}

	operation = NewOperation(outB)
	return mo.Ok(operation), mo.Ok(baseOperation), nil
}

// TransformMulti 对多个操作进行转换
func (t *Transformer) TransformMulti(
	operation *Operation, baseOperation *OperationComponent,
) (newOp mo.Result[*Operation], components mo.Result[*OperationComponent], err error) {
	var out []*OperationComponent
	base := baseOperation.NotNoop()
	for item := operation.Operations.Front(); item != nil; item = item.Next() {
		op, _ := item.Value.(*OperationComponent)
		if base.IsAbsent() { // 如果 base 是 noop， 则直接返回 op
			out = append(out, op)
			continue
		}
		a, err := t.TransformComponent(op, base.MustGet(), TransformSideLeft)
		if err != nil {
			return mo.Err[*Operation](err), mo.Err[*OperationComponent](err), err
		}
		b, err := t.TransformComponent(base.MustGet(), op, TransformSideRight)
		if err != nil {
			return mo.Err[*Operation](err), mo.Err[*OperationComponent](err), err
		}
		if b.IsError() {
			return mo.Err[*Operation](b.Error()), mo.Err[*OperationComponent](b.Error()), b.Error()
		}
		bOps := b.MustGet()
		if len(bOps) <= 1 {
			break
		}

		// 执行 base = b.pop
		base = mo.Some(b.MustGet()[0])
		out = append(out, a.MustGet()...)
	}

	operation = NewOperation(out)
	return mo.Ok(operation), mo.Ok(baseOperation), nil
}

// TransformComponent 对一个操作组件进行转换
func (t *Transformer) TransformComponent(
	newOp *OperationComponent, baseOp *OperationComponent, side TransformSide,
) (mo.Result[[]*OperationComponent], error) {
	if IsEquivalentToNoop(newOp) || IsEquivalentToNoop(baseOp) {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	maxCommonPath := baseOp.Path.MaxCommonPath(newOp.Path)
	newOperatePathLen := newOp.OperatePathLen()
	baseOperatePathLen := baseOp.OperatePathLen()

	if maxCommonPath.Len() < newOperatePathLen &&
		maxCommonPath.Len() < baseOperatePathLen {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	// such as:
	// new_op, base_op
	// [p1,p2,p3], [p1,p2,p4,p5]
	// [p1,p2,p3], [p1,p2,p3,p5]
	if baseOperatePathLen > newOperatePathLen {
		// 如果 base_op 的路径更长并包含 new_op 的路径，则 new_op 应该包含 base_op 的效果
		if newOp.Path.IsPrefixOf(baseOp.Path) {
			t.Consume(newOp, maxCommonPath, baseOp)
		}
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	// 从这里开始，base_op 的路径更短或等于 new_op，例如：
	// new_op, base_op
	// [p1,p2,p3], [p1,p2,p3]. same operand and base_op is prefix of new_op
	// [p1,p2,p4], [p1,p2,p3]. same operand
	// [p1,p2,p3,p4,..], [p1,p2,p3], base_op is prefix of new_op
	// [p1,p2,p4,p5,..], [p1,p2,p3]
	return t.TransformComponentOperator(newOp, baseOp, side)
}

// TransformComponentOperator 对一个操作组件的操作数进行转换
func (t *Transformer) TransformComponentOperator(
	newOp *OperationComponent, baseOp *OperationComponent, side TransformSide,
) (mo.Result[[]*OperationComponent], error) {
	switch bop := baseOp.Operator.(type) {
	case *SubTypeOperator:
		return t.TransformComponentForSubType(newOp, baseOp, side, bop)
	case *ListReplace:
		return t.TransformComponentForListReplace(newOp, baseOp, side, bop)
	case *ListInsert:
		return t.TransformComponentForListInsert(newOp, baseOp, side, bop)
	case *ListDelete:
		return t.TransformComponentForListDelete(newOp, baseOp, side, bop)
	case *ListMove:
		return t.TransformComponentForListMove(newOp, baseOp, side, bop)
	case *ObjectReplace:
		return t.TransformComponentForObjectReplace(newOp, baseOp, side, bop)
	case *ObjectInsert:
		return t.TransformComponentForObjectInsert(newOp, baseOp, side, bop)
	case *ObjectDelete:
		return t.TransformComponentForObjectDelete(newOp, baseOp, side, bop)
	}
	return mo.Ok([]*OperationComponent{newOp}), nil
}

// Consume 消费一个操作
func (t *Transformer) Consume(
	op *OperationComponent,
	commonPath *Path, other *OperationComponent,
) {
	switch v := op.Operator.(type) {
	case *ListDelete:
		_, p2 := other.Path.SplitAt(commonPath.Len())
		_ = ApplyToValue(v.OlvValue, p2, other.Operator)
	case *ListReplace:
		_, p2 := other.Path.SplitAt(commonPath.Len())
		log.Debugf("before apply spited value:%s", v.OldValue.RawMessage())
		_ = ApplyToValue(v.OldValue, p2, other.Operator)
		log.Debugf("after apply spited value:%s with operator: %s", v.OldValue.RawMessage(), other.ToValue().RawMessage())
	case *ObjectDelete:
		_, p2 := other.Path.SplitAt(commonPath.Len())
		_ = ApplyToValue(v.OldValue, p2, other.Operator)
	case *ObjectReplace:
		_, p2 := other.Path.SplitAt(commonPath.Len())
		_ = ApplyToValue(v.OldValue, p2, other.Operator)
	}
}

// TransformComponentForSubType 对一个操作组件进行子类型转换
func (t *Transformer) TransformComponentForSubType(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	baseOperator *SubTypeOperator,
) (mo.Result[[]*OperationComponent], error) {
	newOpAsSubType, ok := newOp.Operator.(*SubTypeOperator)
	if !ok {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	// 如果 baseOperator 的子类型与 newOp 的子类型不同，则直接返回 newOp
	if baseOperator.SubType != newOpAsSubType.SubType {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	ocs := baseOperator.SubTypeFunctions.Transform(newOpAsSubType.Value, baseOperator.Value, side)
	if ocs.IsError() {
		return mo.Err[[]*OperationComponent](ocs.Error()), nil
	}

	var result []*OperationComponent
	for _, components := range ocs.MustGet() {
		newOC := NewOperationComponent(
			baseOp.Path, NewSubTypeOperator(baseOperator.SubType, components, baseOperator.SubTypeFunctions),
		)
		if newOC.IsError() {
			return mo.Err[[]*OperationComponent](newOC.Error()), nil
		}
		result = append(result, newOC.MustGet())
	}
	return mo.Ok(result), nil
}

// TransformComponentForListReplace 对一个 ListReplace 操作进行转换
func (t *Transformer) TransformComponentForListReplace(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	listReplace *ListReplace,
) (mo.Result[[]*OperationComponent], error) {
	if !baseOp.Path.IsPrefixOf(newOp.Path) {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}
	if !IsSameOperand(baseOp, newOp) {
		return mo.Ok([]*OperationComponent{}), nil
	}

	if nop, ok := newOp.Operator.(*ListReplace); ok {
		if side == TransformSideLeft {
			return mo.Ok([]*OperationComponent{
				NewOperationComponent(newOp.Path,
					NewListReplace(nop.NewValue, listReplace.NewValue)).MustGet(),
			}), nil
		}

		return mo.Ok([]*OperationComponent{}), nil
	}

	if _, ok := newOp.Operator.(*ListDelete); ok {
		return mo.Ok([]*OperationComponent{}), nil
	}

	return mo.Ok([]*OperationComponent{newOp}), nil
}

// TransformComponentForListInsert 对一个 ListInsert 操作进行转换
func (t *Transformer) TransformComponentForListInsert(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	_ *ListInsert,
) (mo.Result[[]*OperationComponent], error) {
	sameOperand := IsSameOperand(baseOp, newOp)
	baseOpIsPrefix := baseOp.Path.IsPrefixOf(newOp.Path)

	if _, ok := newOp.Operator.(*ListInsert); ok {
		if sameOperand && baseOpIsPrefix {
			if side == TransformSideRight {
				newOp.Path.IncreaseIndex(baseOp.OperatePathLen())
			}
			return mo.Ok([]*OperationComponent{newOp}), nil
		}
	}

	basePath := baseOp.Path.Get(baseOp.OperatePathLen())
	newPath := newOp.Path.Get(baseOp.OperatePathLen())
	if basePath.IsPresent() && newPath.IsPresent() {
		if p1, p2 := basePath.MustGet(), newPath.MustGet(); p1.Index <= p2.Index {
			newOp.Path.IncreaseIndex(baseOp.OperatePathLen())
		}
	}

	if lm, ok := newOp.Operator.(*ListMove); ok {
		if sameOperand && basePath.IsPresent() {
			if p := basePath.MustGet(); p.Index <= lm.NewIndex {
				newOp.Operator = NewListMove(lm.NewIndex + 1)
			}
		}
	}

	return mo.Ok([]*OperationComponent{newOp}), nil
}

// TransformComponentForListDelete 对一个 ListDelete 操作进行转换
func (t *Transformer) TransformComponentForListDelete(
	newOp, baseOp *OperationComponent,
	_ TransformSide,
	_ *ListDelete,
) (mo.Result[[]*OperationComponent], error) {
	baseOpOperatePath := baseOp.Path.Get(baseOp.OperatePathLen()).MustGet()
	newOpOperatePath := newOp.Path.Get(baseOp.OperatePathLen()).MustGet()
	if lm, ok := newOp.Operator.(*ListMove); ok {
		if IsSameOperand(baseOp, newOp) {
			if baseOp.Path.IsPrefixOf(newOp.Path) {
				return mo.Ok([]*OperationComponent{}), nil
			}
			to := lm.NewIndex
			if baseOpOperatePath.Index < to || (baseOpOperatePath.Index == to && newOpOperatePath.Index < to) {
				newOp.Operator = NewListMove(lm.NewIndex - 1)
			}
		}
	}

	if baseOpOperatePath.Index < newOpOperatePath.Index {
		newOp.Path.DecreaseIndex(baseOp.OperatePathLen())
	} else if baseOp.Path.IsPrefixOf(newOp.Path) {
		if !IsSameOperand(baseOp, newOp) {
			return mo.Ok([]*OperationComponent{}), nil
		}
		if _, ok := newOp.Operator.(*ListDelete); ok {
			return mo.Ok([]*OperationComponent{}), nil
		}
		if lr, ok := newOp.Operator.(*ListReplace); ok {
			return mo.Ok([]*OperationComponent{
				NewOperationComponent(newOp.Path, NewListInsert(lr.NewValue)).MustGet(),
			}), nil
		}
	}

	return mo.Ok([]*OperationComponent{newOp}), nil
}

// TransformComponentForObjectReplace 对一个 ObjectReplace 操作进行转换
func (t *Transformer) TransformComponentForObjectReplace(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	objectReplace *ObjectReplace,
) (mo.Result[[]*OperationComponent], error) {
	if !baseOp.Path.IsPrefixOf(newOp.Path) {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	if !IsSameOperand(baseOp, newOp) {
		return mo.Ok([]*OperationComponent{}), nil
	}

	switch nop := newOp.Operator.(type) {
	case *ObjectReplace:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(newOp.Path, NewObjectReplace(nop.NewValue, objectReplace.NewValue)).MustGet(),
		}), nil
	case *ObjectInsert:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(newOp.Path, NewObjectReplace(nop.NewValue, objectReplace.NewValue)).MustGet(),
		}), nil
	default:
		return mo.Ok([]*OperationComponent{}), nil
	}
}

// TransformComponentForObjectInsert 对一个 ObjectInsert 操作进行转换
func (t *Transformer) TransformComponentForObjectInsert(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	objectInsert *ObjectInsert,
) (mo.Result[[]*OperationComponent], error) {
	if !baseOp.Path.IsPrefixOf(newOp.Path) {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	switch nop := newOp.Operator.(type) {
	case *ObjectReplace:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		if IsSameOperand(baseOp, newOp) {
			return mo.Ok([]*OperationComponent{
				NewOperationComponent(baseOp.Path, NewObjectReplace(nop.NewValue, objectInsert.NewValue)).MustGet(),
			}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(baseOp.Path, NewObjectDelete(objectInsert.NewValue)).MustGet(),
			newOp,
		}), nil
	case *ObjectInsert:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		if IsSameOperand(baseOp, newOp) {
			return mo.Ok([]*OperationComponent{
				NewOperationComponent(baseOp.Path, NewObjectReplace(nop.NewValue, objectInsert.NewValue)).MustGet(),
			}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(baseOp.Path, NewObjectDelete(objectInsert.NewValue)).MustGet(),
			newOp,
		}), nil
	case *ObjectDelete:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		return mo.Ok([]*OperationComponent{newOp}), nil
	}
	return mo.Ok([]*OperationComponent{newOp}), nil
}

// TransformComponentForObjectDelete 对一个 ObjectDelete 操作进行转换
func (t *Transformer) TransformComponentForObjectDelete(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	_ *ObjectDelete,
) (mo.Result[[]*OperationComponent], error) {
	if !baseOp.Path.IsPrefixOf(newOp.Path) {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	// 如果是不同的操作数，则直接返回空
	if !IsSameOperand(baseOp, newOp) {
		return mo.Ok([]*OperationComponent{}), nil
	}

	switch nop := newOp.Operator.(type) {
	case *ObjectReplace:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(newOp.Path, NewObjectInsert(nop.NewValue)).MustGet(),
		}), nil
	case *ObjectInsert:
		if side == TransformSideRight {
			return mo.Ok([]*OperationComponent{}), nil
		}
		return mo.Ok([]*OperationComponent{
			NewOperationComponent(newOp.Path, NewObjectInsert(nop.NewValue)).MustGet(),
		}), nil
	default:
		return mo.Ok([]*OperationComponent{}), nil
	}
}

// TransformComponentForListMove 对一个 ListMove 操作进行转换
func (t *Transformer) TransformComponentForListMove(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	listMove *ListMove,
) (mo.Result[[]*OperationComponent], error) {
	if IsSameOperand(baseOp, newOp) {
		switch newOperator := newOp.Operator.(type) {
		case *ListMove:
			return t.TransformComponentForListMoveWithListMove(newOp, baseOp, side, listMove, newOperator)
		case *ListInsert:
			return t.TransformComponentForListMoveWithListInsert(newOp, baseOp, side, listMove, newOperator)
		}
	}

	from := baseOp.Path.Get(baseOp.OperatePathLen()).MustGet()
	to := PathElement{Index: listMove.NewIndex}

	p := newOp.Path.Get(baseOp.OperatePathLen()).MustGet()

	if p.Index == from.Index {
		newOp.Path.Replace(baseOp.OperatePathLen(), to)
	} else {
		if p.Index > from.Index {
			newOp.Path.DecreaseIndex(baseOp.OperatePathLen())
		}
		if p.Index > to.Index || (p.Index == to.Index && from.Index > to.Index) {
			newOp.Path.IncreaseIndex(baseOp.OperatePathLen())
		}
	}

	return mo.Ok([]*OperationComponent{newOp}), nil
}

// TransformComponentForListMoveWithListMove 对一个 ListMove 操作进行转换
func (t *Transformer) TransformComponentForListMoveWithListMove(
	newOp, baseOp *OperationComponent,
	side TransformSide,
	listMove, newListMove *ListMove,
) (mo.Result[[]*OperationComponent], error) {
	baseOpOperatePathLen := baseOp.OperatePathLen()
	newOpOperatePathLen := newOp.OperatePathLen()
	otherFrom := baseOp.Path.Get(newOpOperatePathLen).MustGet()
	otherTo := PathElement{Index: listMove.NewIndex}

	if otherFrom == otherTo {
		return mo.Ok([]*OperationComponent{newOp}), nil
	}

	from := newOp.Path.Get(newOpOperatePathLen).MustGet()
	to := PathElement{Index: newListMove.NewIndex}
	if from == otherFrom {
		if to == otherTo {
			// already moved to where we want
			return mo.Ok([]*OperationComponent{}), nil
		}

		if side == TransformSideLeft {
			newOp.Path.Replace(baseOpOperatePathLen, otherTo)
			if from == to {
				newOp.Operator = baseOp.Operator
			}
			return mo.Ok([]*OperationComponent{newOp}), nil
		}

		return mo.Ok([]*OperationComponent{}), nil
	}

	newListMoveIndex := newListMove.NewIndex
	newListMoveIndex = t.mergeListMoveIndex(
		newOp, newListMoveIndex, baseOpOperatePathLen, side, from, to, otherFrom, otherTo,
	)

	newOp.Operator = NewListMove(newListMoveIndex)
	return mo.Ok([]*OperationComponent{newOp}), nil
}

// mergeListMoveIndex 合并 ListMove 的索引
func (t *Transformer) mergeListMoveIndex(
	newOp *OperationComponent,
	newListMoveIndex int,
	baseOpOperatePathLen int,
	side TransformSide,
	from PathElement,
	to PathElement,
	otherFrom PathElement,
	otherTo PathElement,
) int {
	if from.Index > otherFrom.Index {
		newOp.Path.DecreaseIndex(baseOpOperatePathLen)
	}
	if from.Index > otherTo.Index {
		newOp.Path.IncreaseIndex(baseOpOperatePathLen)
	} else if from.Index == otherTo.Index && otherFrom.Index > otherTo.Index {
		newOp.Path.IncreaseIndex(baseOpOperatePathLen)
		if from.Index == to.Index {
			newListMoveIndex++
		}
	}

	if to.Index > otherFrom.Index || (to.Index == otherFrom.Index && to.Index > from.Index) {
		newListMoveIndex--
	}

	if to.Index > otherTo.Index {
		newListMoveIndex++
	}

	if to.Index == otherTo.Index {
		if (otherTo.Index > otherFrom.Index && to.Index > from.Index) ||
			(otherTo.Index < otherFrom.Index && to.Index < from.Index) {
			if side == TransformSideRight {
				newListMoveIndex++
			}
		} else if to.Index > from.Index {
			newListMoveIndex++
		} else if to.Index == otherFrom.Index {
			newListMoveIndex--
		}
	}

	return newListMoveIndex
}

// TransformComponentForListMoveWithListInsert 对一个 ListMove 操作进行转换
func (t *Transformer) TransformComponentForListMoveWithListInsert(
	newOp, baseOp *OperationComponent,
	_ TransformSide,
	listMove *ListMove,
	_ *ListInsert,
) (mo.Result[[]*OperationComponent], error) {
	operateIndex := baseOp.OperatePathLen()
	from := baseOp.Path.Get(operateIndex).MustGet()
	to := listMove.NewIndex

	p := newOp.Path.Get(operateIndex).MustGet()
	if p.Index > from.Index {
		newOp.Path.DecreaseIndex(operateIndex)
	}
	if p.Index > to {
		newOp.Path.IncreaseIndex(operateIndex)
	}

	return mo.Ok([]*OperationComponent{newOp}), nil
}
