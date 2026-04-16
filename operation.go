package jsonot

import (
	"container/list"
	"fmt"

	"github.com/samber/mo"
)

// OperationComponent is a component of an operation.
type OperationComponent struct {
	Operator Operator
	Path     Path
}

// NewOperationComponent creates a new operation component.
func NewOperationComponent(path Path, operator Operator) mo.Result[*OperationComponent] {
	op := &OperationComponent{Path: path.Clone(), Operator: operator.Clone()}
	return mo.Ok(op)
}

// Format formats the operation component according to the fmt.Formatter interface.
func (oc *OperationComponent) Format(st fmt.State, _ rune) {
	if oc.Path.IsEmpty() {
		_, _ = fmt.Fprintf(st, "Path:Noop")
		return
	}

	if _, ok := oc.Operator.(Noop); ok {
		_, _ = fmt.Fprintf(st, "Path:%v: Noop", oc.Path)
		return
	}

	_, _ = fmt.Fprintf(st, "Path: %v: %+v", oc.Path, oc.Operator)
}

// ToValue converts the operation component to a Value.
func (oc *OperationComponent) ToValue() Value {
	obj := make(map[string]interface{})
	obj["p"] = oc.Path.ToValue().RawMessage()
	switch op := oc.Operator.(type) {
	case *ListDelete:
		obj[string(ActionListDelete)] = op.OlvValue.RawMessage()
	case *ListInsert:
		obj[string(ActionListInsert)] = op.NewValue.RawMessage()
	case *ListReplace:
		obj[string(ActionListInsert)] = op.NewValue.RawMessage()
		obj[string(ActionListDelete)] = op.OldValue.RawMessage()
	case *ListMove:
		obj[string(ActionListMove)] = op.NewIndex
	case *ObjectInsert:
		obj[string(ActionObjectInsert)] = op.NewValue.RawMessage()
	case *ObjectDelete:
		obj[string(ActionObjectDelete)] = op.OldValue.RawMessage()
	case *ObjectReplace:
		obj[string(ActionObjectInsert)] = op.NewValue.RawMessage()
		obj[string(ActionObjectDelete)] = op.OldValue.RawMessage()
	case *SubTypeOperator:
		switch st := op.SubType.(type) {
		case *NumberAdd:
			obj[string(ActionSubTypeNumberAdd)] = op.Value.RawMessage()
		default:
			obj[string(ActionSubType)] = st.TypeName()
			obj[SubTypeOperand] = op.Value.RawMessage()
		}
	}

	return ValueFromAny(obj)
}

// Noop creates a noop operation component.
func (oc *OperationComponent) Noop() *OperationComponent {
	return &OperationComponent{Path: oc.Path, Operator: Noop{}}
}

// Clone clones the operation component.
func (oc *OperationComponent) Clone() *OperationComponent {
	return &OperationComponent{
		Path:     oc.Path.Clone(),
		Operator: oc.Operator.Clone(),
	}
}

// CloneNotNoop clones the operation component if it is not a noop.
func (oc *OperationComponent) CloneNotNoop() mo.Option[*OperationComponent] {
	if _, ok := oc.Operator.(Noop); ok {
		return mo.None[*OperationComponent]()
	}
	return mo.Some(oc)
}

// NotNoop returns the operation component if it is not a noop.
func (oc *OperationComponent) NotNoop() mo.Option[*OperationComponent] {
	if _, ok := oc.Operator.(Noop); ok {
		return mo.None[*OperationComponent]()
	}
	return mo.Some(oc)
}

// Invert inverts the operation component.
func (oc *OperationComponent) Invert() mo.Result[*OperationComponent] {
	path := oc.Path
	var operator Operator
	switch op := oc.Operator.(type) {
	case *Noop:
		operator = &Noop{}
	case *SubTypeOperator:
		operator, _ = op.SubTypeFunctions.Invert(op.SubType, path, op.Value).Get()
	case *ListInsert:
		operator = NewListDelete(op.NewValue)
	case *ListDelete:
		operator = NewListInsert(op.OlvValue)
	case *ListReplace:
		operator = NewListReplace(op.OldValue, op.NewValue)
	case *ListMove:
		oldPath := path.Replace(path.Len()-1, PathElementFromIndex(op.NewIndex))
		if oldPath.IsPresent() && oldPath.MustGet().Index != 0 {
			operator = NewListMove(oldPath.MustGet().Index)
		} else {
			return mo.Err[*OperationComponent](NewError(BadPath).Append("list move operation path is invalid"))
		}
	case *ObjectInsert:
		operator = NewObjectDelete(op.NewValue)
	case *ObjectDelete:
		operator = NewObjectInsert(op.OldValue)
	case *ObjectReplace:
		operator = NewObjectReplace(op.OldValue, op.NewValue)
	}
	return NewOperationComponent(path, operator)
}

// Merge merges the operation component with another operation component.
func (oc *OperationComponent) Merge(op *OperationComponent) mo.Option[*OperationComponent] {
	var newOp mo.Option[Operator]
	switch v1 := oc.Operator.(type) {
	case *Noop:
		newOp = mo.Some[Operator](NewNoop())
	case *SubTypeOperator:
		newOp = v1.SubTypeFunctions.Merge(v1.Value, op.Operator)
	case *ListInsert:
		newOp = oc.MergeListInsert(op, v1)
	case *ListReplace:
		newOp = oc.MergeListReplace(op, v1)
	case *ObjectInsert:
		newOp = oc.MergeObjectInsert(op, v1)
	case *ObjectDelete:
		switch v2 := op.Operator.(type) {
		case *ObjectInsert:
			newOp = mo.Some[Operator](NewObjectReplace(v2.NewValue, v1.OldValue))
		default:
			newOp = mo.None[Operator]()
		}
	case *ObjectReplace:
		newOp = oc.MergeObjectReplace(op, v1)
	}

	if newOp.IsAbsent() {
		return mo.None[*OperationComponent]()
	}

	newOc := &OperationComponent{Path: oc.Path, Operator: newOp.MustGet()}
	return mo.Some(newOc)
}

// MergeListInsert merges a ListInsert operation with another operation.
func (oc *OperationComponent) MergeListInsert(op *OperationComponent, v1 *ListInsert) mo.Option[Operator] {
	var newOp mo.Option[Operator]
	switch v2 := op.Operator.(type) {
	case *ListDelete:
		if v1.NewValue.Equals(v2.OlvValue) {
			newOp = mo.Some[Operator](NewNoop())
		} else {
			newOp = mo.None[Operator]()
		}
	case *ListReplace:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewListInsert(v2.NewValue))
		} else {
			newOp = mo.None[Operator]()
		}
	default:
		newOp = mo.None[Operator]()
	}

	return newOp
}

// MergeListReplace merges a ListReplace operation with another operation.
func (oc *OperationComponent) MergeListReplace(op *OperationComponent, v1 *ListReplace) mo.Option[Operator] {
	var newOp mo.Option[Operator]
	switch v2 := op.Operator.(type) {
	case *ListDelete:
		if v1.NewValue == v2.OlvValue {
			newOp = mo.Some[Operator](NewListDelete(v1.OldValue))
		} else {
			newOp = mo.None[Operator]()
		}
	case *ListReplace:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewListReplace(v2.NewValue, v1.OldValue))
		} else {
			newOp = mo.None[Operator]()
		}
	default:
		newOp = mo.None[Operator]()
	}
	return newOp
}

// MergeObjectInsert merges an ObjectInsert operation with another operation.
func (oc *OperationComponent) MergeObjectInsert(op *OperationComponent, v1 *ObjectInsert) mo.Option[Operator] {
	var newOp mo.Option[Operator]
	switch v2 := op.Operator.(type) {
	case *ObjectDelete:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewNoop())
		} else {
			newOp = mo.None[Operator]()
		}
	case *ObjectReplace:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewObjectInsert(v2.NewValue))
		} else {
			newOp = mo.None[Operator]()
		}
	default:
		newOp = mo.None[Operator]()
	}
	return newOp
}

// MergeObjectReplace merges an ObjectReplace operation with another operation.
func (oc *OperationComponent) MergeObjectReplace(op *OperationComponent, v1 *ObjectReplace) mo.Option[Operator] {
	var newOp mo.Option[Operator]
	switch v2 := op.Operator.(type) {
	case *ObjectDelete:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewObjectDelete(v1.OldValue))
		} else {
			newOp = mo.None[Operator]()
		}
	case *ObjectReplace:
		if v1.NewValue.Equals(v2.OldValue) {
			newOp = mo.Some[Operator](NewObjectReplace(v2.NewValue, v1.OldValue))
		} else {
			newOp = mo.None[Operator]()
		}
	default:
		newOp = mo.None[Operator]()
	}

	return newOp
}

// OperatePathLen returns the length of the path of the operation component.
func (oc *OperationComponent) OperatePathLen() int {
	switch oc.Operator.(type) {
	case *SubTypeOperator:
		return oc.Path.Len()
	default:
		p := oc.Path
		return p.Len() - 1
	}
}

// Validation validates the operation component.
func (oc *OperationComponent) Validation() error {
	if oc.Path.IsEmpty() && !supportsRootPathOperator(oc.Operator) {
		return NewError(BadPath).Append("component path is empty")
	}

	return oc.Operator.Validates()
}

func supportsRootPathOperator(operator Operator) bool {
	switch operator.(type) {
	case *Noop, *ListInsert, *ListDelete, *ListReplace, *ObjectInsert, *ObjectDelete, *ObjectReplace, *SubTypeOperator:
		return true
	default:
		return false
	}
}

// NewOperation creates a new operation.
func NewOperation(operations []*OperationComponent) *Operation {
	op := &Operation{Operations: list.New()}
	for _, o := range operations {
		op.Operations.PushBack(o)
	}
	return op
}

// EmptyOperation creates an empty operation.
func EmptyOperation() *Operation {
	return &Operation{}
}

// Operation 一个操作由多个操作组件组成
type Operation struct {
	Operations *list.List
}

// Format returns a string representation of the operation.
func (o *Operation) Format(st fmt.State, verb rune) {
	if verb != 'v' {
		return
	}

	for e := o.Operations.Front(); e != nil; e = e.Next() {
		op, _ := e.Value.(*OperationComponent)
		if op != nil {
			_, _ = fmt.Fprintf(st, "\n%+v", op)
		}
	}
}

// ToValue converts the operation to a Value.
func (o *Operation) ToValue() Value {
	var components []Value
	if o.Operations == nil || o.Operations.Len() == 0 {
		return ValueFromArray(components)
	}

	for e := o.Operations.Front(); e != nil; e = e.Next() {
		op, _ := e.Value.(*OperationComponent)
		if op != nil {
			components = append(components, op.ToValue())
		}
	}

	return ValueFromArray(components)
}

// Append appends an operation component to the operation.
func (o *Operation) Append(op *OperationComponent) {
	switch m := op.Operator.(type) {
	case *ListMove:
		lastPath := op.Path.Get(op.Path.Len() - 1)
		if lastPath.IsAbsent() {
			break
		}
		if lastPath.MustGet().Index == m.NewIndex {
			return
		}
	}
	if o.Operations.Len() == 0 {
		o.Operations.PushBack(op)
		return
	}

	last := o.Operations.Back()
	lastOC, _ := o.Operations.Back().Value.(*OperationComponent)
	// 检查新合入的操作是否可以合并到最后一个操作中？这是因为？？
	if lastOC.Path.Equal(op.Path) {
		if newOp := lastOC.Merge(op); newOp.IsPresent() {
			o.Operations.Remove(last)
			o.Operations.PushBack(newOp.MustGet())
		} else {
			if _, ok := lastOC.Operator.(Noop); ok {
				o.Operations.Remove(o.Operations.Back())
			} else {
				o.Operations.PushBack(op)
			}
		}
	} else {
		o.Operations.PushBack(op)
	}
}

// Compose composes the operation with another operation.
func (o *Operation) Compose(other *Operation) {
	for e := other.Operations.Front(); e != nil; e = e.Next() {
		op, _ := e.Value.(*OperationComponent)
		o.Append(op)
	}
}

// Validation validates the operation.
func (o *Operation) Validation() error {
	for e := o.Operations.Front(); e != nil; e = e.Next() {
		op, _ := e.Value.(*OperationComponent)
		if err := op.Validation(); err != nil {
			return err
		}
	}
	return nil
}

// IsEmpty returns true if the operation is empty.
func (o *Operation) IsEmpty() bool {
	return o.Operations.Len() == 0
}

// Len returns the length of the operation.
func (o *Operation) Len() int {
	return o.Operations.Len()
}

// Array returns the operation as an array of
func (o *Operation) Array() []*OperationComponent {
	var components []*OperationComponent
	for e := o.Operations.Front(); e != nil; e = e.Next() {
		op, _ := e.Value.(*OperationComponent)
		components = append(components, op)
	}

	return components
}

// ListOperationBuilder is a builder for list operations.
type ListOperationBuilder struct {
	insert mo.Option[Value]
	delete mo.Option[Value]
	path   Path
	moveTo mo.Option[int]
}

// NewListOperationBuilder creates a new list operation builder.
func NewListOperationBuilder(path Path) *ListOperationBuilder {
	return &ListOperationBuilder{path: path}
}

// Insert sets the insert value.
func (b *ListOperationBuilder) Insert(val Value) *ListOperationBuilder {
	b.insert = mo.Some(val)
	return b
}

// Delete sets the delete value.
func (b *ListOperationBuilder) Delete(val Value) *ListOperationBuilder {
	b.delete = mo.Some(val)
	return b
}

// Replace sets the replacement values.
func (b *ListOperationBuilder) Replace(oldVal, newVal Value) *ListOperationBuilder {
	b.insert = mo.Some(newVal)
	b.delete = mo.Some(oldVal)
	return b
}

// MoveTo sets the new index.
func (b *ListOperationBuilder) MoveTo(newIndex int) *ListOperationBuilder {
	b.moveTo = mo.Some(newIndex)
	return b
}

// Build builds the operation component.
func (b *ListOperationBuilder) Build() mo.Result[*OperationComponent] {
	if b.moveTo.IsPresent() {
		return NewOperationComponent(b.path, &ListMove{NewIndex: b.moveTo.MustGet()})
	}

	if b.delete.IsPresent() {
		if b.insert.IsPresent() {
			return NewOperationComponent(b.path, &ListReplace{
				NewValue: b.insert.MustGet(), OldValue: b.delete.MustGet(),
			})
		}
		return NewOperationComponent(b.path, &ListDelete{OlvValue: b.delete.MustGet()})
	}

	if b.insert.IsPresent() {
		return NewOperationComponent(b.path, &ListInsert{NewValue: b.insert.MustGet()})
	}

	return NewOperationComponent(b.path, &Noop{})
}

// ObjectOperationBuilder is a builder for object operations.
type ObjectOperationBuilder struct {
	insert  mo.Option[Value]
	delete  mo.Option[Value]
	replace mo.Option[Value]
	path    Path
}

// NewObjectOperationBuilder creates a new object operation builder.
func NewObjectOperationBuilder(path Path) *ObjectOperationBuilder {
	return &ObjectOperationBuilder{path: path}
}

// Insert sets the insert value.
func (b *ObjectOperationBuilder) Insert(val Value) *ObjectOperationBuilder {
	b.insert = mo.Some(val)
	return b
}

// Delete sets the delete value.
func (b *ObjectOperationBuilder) Delete(val Value) *ObjectOperationBuilder {
	b.delete = mo.Some(val)
	return b
}

// Replace sets the replacement values.
func (b *ObjectOperationBuilder) Replace(oldVal, newVal Value) *ObjectOperationBuilder {
	b.replace = mo.Some(newVal)
	b.delete = mo.Some(oldVal)
	return b
}

// Build builds the operation component.
func (b *ObjectOperationBuilder) Build() mo.Result[*OperationComponent] {
	if b.delete.IsPresent() {
		if b.insert.IsPresent() {
			return NewOperationComponent(b.path, &ObjectReplace{
				NewValue: b.insert.MustGet(), OldValue: b.delete.MustGet(),
			})
		}
		return NewOperationComponent(b.path, &ObjectDelete{OldValue: b.delete.MustGet()})
	}

	if b.insert.IsPresent() {
		return NewOperationComponent(b.path, &ObjectInsert{NewValue: b.insert.MustGet()})
	}

	return NewOperationComponent(b.path, &Noop{})
}

// NumberAddOperationBuilder is a builder for number add operations.
type NumberAddOperationBuilder struct {
	subTypeFunctions SubTypeFunctions
	path             Path
	numberInt        mo.Option[int64]
	numberFlt        mo.Option[float64]
}

// NewNumberAddOperationBuilder creates a new number add operation builder.
func NewNumberAddOperationBuilder(path Path, subTypeFunctions SubTypeFunctions) *NumberAddOperationBuilder {
	return &NumberAddOperationBuilder{path: path, subTypeFunctions: subTypeFunctions}
}

// AddInt sets the add value as an integer.
func (b *NumberAddOperationBuilder) AddInt(val int64) *NumberAddOperationBuilder {
	b.numberInt = mo.Some(val)
	return b
}

// AddFloat sets the add value as a float.
func (b *NumberAddOperationBuilder) AddFloat(val float64) *NumberAddOperationBuilder {
	b.numberFlt = mo.Some(val)
	return b
}

// Build builds the operation component.
func (b *NumberAddOperationBuilder) Build() mo.Result[*OperationComponent] {
	if b.numberInt.IsPresent() {
		return NewOperationComponent(b.path,
			NewSubTypeOperator(NewNumberAdd(), ValueFromPrimitive(b.numberInt.MustGet()), b.subTypeFunctions))
	}
	if b.numberFlt.IsAbsent() {
		return NewOperationComponent(b.path,
			NewSubTypeOperator(NewNumberAdd(), ValueFromPrimitive(b.numberFlt.MustGet()), b.subTypeFunctions))
	}

	return mo.Err[*OperationComponent](NewError(InvalidOperation).Append("na operand value is required"))
}

// TextOperationBuilder is a builder for text operations.
type TextOperationBuilder struct {
	subTypeFunction SubTypeFunctions
	insertVal       mo.Option[string]
	deleteVal       mo.Option[string]
	path            Path
	offset          int
}

// NewTextOperationBuilder creates a new text operation builder.
func NewTextOperationBuilder(path Path, subTypeFunction SubTypeFunctions) *TextOperationBuilder {
	return &TextOperationBuilder{
		path:            path,
		insertVal:       mo.None[string](),
		deleteVal:       mo.None[string](),
		subTypeFunction: subTypeFunction,
	}
}

// InsertStr sets the insert value as a string.
func (b *TextOperationBuilder) InsertStr(offset int, val mo.Option[string]) *TextOperationBuilder {
	b.offset = offset
	b.insertVal = val
	return b
}

// DeleteStr sets the delete value as a string.
func (b *TextOperationBuilder) DeleteStr(offset int, val mo.Option[string]) *TextOperationBuilder {
	b.offset = offset
	b.deleteVal = val
	return b
}

// Build builds the operation component for text operations.
func (b *TextOperationBuilder) Build() mo.Result[*OperationComponent] {
	if (b.insertVal.IsAbsent() && b.deleteVal.IsAbsent()) || (b.insertVal.IsPresent() && b.deleteVal.IsPresent()) {
		return mo.Err[*OperationComponent](NewError(InvalidOperation).Append("text operand value invalid"))
	}

	m := map[string]any{}
	m[TextOperandOffsetKey] = b.offset
	if b.insertVal.IsPresent() {
		m[TextOperandInsertKey] = b.insertVal.MustGet()
	} else {
		m[TextOperandDeleteKey] = b.deleteVal.MustGet()
	}

	return NewOperationComponent(b.path, NewSubTypeOperator(NewText(), ValueFromAny(m), b.subTypeFunction))
}

// SubTypeOperationBuilder is a builder for subtype operations.
type SubTypeOperationBuilder struct {
	subType         SubType
	subTypeOperand  Value
	subTypeFunction SubTypeFunctions
	path            Path
}

// NewSubTypeOperationBuilder creates a new subtype operation builder.
func NewSubTypeOperationBuilder(path Path, subType SubType, subTypeFunction SubTypeFunctions) *SubTypeOperationBuilder {
	return &SubTypeOperationBuilder{
		path:            path,
		subType:         subType,
		subTypeFunction: subTypeFunction,
	}
}

// SubTypeOperand sets the subtype operand value.
func (b *SubTypeOperationBuilder) SubTypeOperand(val Value) *SubTypeOperationBuilder {
	b.subTypeOperand = val
	return b
}

// SubTypeFunctions sets the subtype functions.
func (b *SubTypeOperationBuilder) SubTypeFunctions(f SubTypeFunctions) *SubTypeOperationBuilder {
	b.subTypeFunction = f
	return b
}

// Build builds the operation component for subtype operations.
func (b *SubTypeOperationBuilder) Build() mo.Result[*OperationComponent] {
	if b.subTypeOperand == nil {
		return mo.Err[*OperationComponent](NewError(InvalidOperation).Append("subtype operand value is required"))
	}
	if b.subTypeFunction == nil {
		return mo.Err[*OperationComponent](NewError(InvalidOperation).Append("subtype function is required"))
	}
	return NewOperationComponent(b.path, NewSubTypeOperator(b.subType, b.subTypeOperand, b.subTypeFunction))
}

type OperationFactory struct {
	subTypeHolder SubTypeFunctionsHolder
}

// NewOperationFactory creates a new operation factory with the given subtype functions holder.
func NewOperationFactory(holder SubTypeFunctionsHolder) *OperationFactory {
	return &OperationFactory{subTypeHolder: holder}
}

// ListOperationBuilder creates a new list operation builder for the given path.
func (f *OperationFactory) ListOperationBuilder(path Path) *ListOperationBuilder {
	return NewListOperationBuilder(path)
}

// ObjectOperationBuilder creates a new object operation builder for the given path.
func (f *OperationFactory) ObjectOperationBuilder(path Path) *ObjectOperationBuilder {
	return NewObjectOperationBuilder(path)
}

// NumberAddOperationBuilder creates a new number add operation builder for the given path.
func (f *OperationFactory) NumberAddOperationBuilder(path Path) *NumberAddOperationBuilder {
	subTypeFunctions := f.subTypeHolder.Get(ActionSubTypeNumberAdd)
	return NewNumberAddOperationBuilder(path, subTypeFunctions.MustGet())
}

// TextOperationBuilder creates a new text operation builder for the given path.
func (f *OperationFactory) TextOperationBuilder(path Path) *TextOperationBuilder {
	subTypeFunctions := f.subTypeHolder.Get(ActionSubTypeText)
	return NewTextOperationBuilder(path, subTypeFunctions.MustGet())
}

// OperationComponentFromValue creates an operation component from a value.
func (f *OperationFactory) OperationComponentFromValue(val Value) mo.Result[*OperationComponent] {
	p := val.GetKey(TextOperandOffsetKey)
	if p.IsAbsent() {
		return mo.Err[*OperationComponent](
			NewError(InvalidOperation).Append("offset is required for text operation"),
		)
	}

	var path Path
	path.FromValue(p.MustGet())
	operator := f.OperatorFromValue(val)
	if operator.IsError() {
		return mo.Err[*OperationComponent](operator.Error())
	}

	return NewOperationComponent(path, operator.MustGet())
}

// OperatorFromValue creates an operator from a value.
func (f *OperationFactory) OperatorFromValue(val Value) mo.Result[Operator] {
	if val.IsObject() {
		return f.MapToOperator(val)
	}

	return mo.Err[Operator](NewError(InvalidOperation).Append("operator value must be an object"))
}

// MapToOperator converts a map to an operator.
func (f *OperationFactory) MapToOperator(obj Value) mo.Result[Operator] {
	// 判断是否是子类型操作
	if obj.HasKey(NumberAddSubTypeName) {
		return f.MapToOperatorForNumberAdd(obj)
	}
	if obj.HasKey(string(ActionSubType)) {
		return f.MapToOperatorForSubType(obj)
	}
	if obj.HasKey(string(ActionListMove)) { // List Move
		return f.MapToOperatorForListMove(obj)
	}
	if obj.HasKey(string(ActionListInsert)) { // List Insert
		if obj.HasKey(string(ActionListDelete)) {
			return f.MapToOperatorForListReplace(obj)
		}
		return f.MapToOperatorForListInsert(obj)
	}
	if obj.HasKey(string(ActionListDelete)) { // List Delete
		return f.MapToOperatorForListDelete(obj)
	}

	if obj.HasKey(string(ActionObjectInsert)) {
		if obj.HasKey(string(ActionObjectDelete)) {
			return f.MapToOperatorForObjectReplace(obj)
		}
		return f.MapToOperatorForObjectInsert(obj)
	}
	if obj.HasKey(string(ActionObjectDelete)) { // Object Delete
		return f.MapToOperatorForObjectDelete(obj)
	}

	result := f.ValidateOperationObjectSize(obj, 1)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	return mo.Ok[Operator](&Noop{}) // 默认返回 Noop 操作
}

// ValidateOperationObjectSize 检查操作对象的大小是否超过限制
func (f *OperationFactory) ValidateOperationObjectSize(val Value, expectSize int) mo.Result[bool] {
	valSize := val.Size()
	if valSize < expectSize {
		return mo.Err[bool](NewError(InvalidParameter).Append("json object size less than operator required"))
	}
	return mo.Ok[bool](true)
}

// MapToOperatorForSubType 自定义子类型操作
func (f *OperationFactory) MapToOperatorForSubType(obj Value) mo.Result[Operator] {
	subTypeKey := obj.GetStringKey(string(ActionSubType))
	result := f.ValidateOperationObjectSize(obj, 3)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	opKey := obj.GetKey(SubTypeOperand)
	op := opKey.OrElse(ValueFromAny(map[string]any{}))

	subType := NewCustom(subTypeKey.MustGet())
	subTypeFunc := f.subTypeHolder.Get(SubTypeAction(subTypeKey.MustGet()))
	if subTypeFunc.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("unsupported sub type: %s", subTypeKey.OrEmpty()))
	}

	return mo.Ok[Operator](NewSubTypeOperator(subType, op, subTypeFunc.MustGet()))
}

// MapToOperatorForNumberAdd converts a map to a number add operator.
func (f *OperationFactory) MapToOperatorForNumberAdd(obj Value) mo.Result[Operator] {
	na := obj.GetKey(NumberAddSubTypeName) // 调用前已经判断存在字段了
	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	val := na.MustGet().GetNumeric()
	if val.IsError() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for na operation invalid"))
	}

	return mo.Ok[Operator](NewSubTypeOperator(
		NewNumberAdd(), na.MustGet(), f.subTypeHolder.Get(ActionSubTypeNumberAdd).MustGet()),
	)
}

// MapToOperatorForListMove 将 map 转换为 ListMove 操作
func (f *OperationFactory) MapToOperatorForListMove(obj Value) mo.Result[Operator] {
	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	index := obj.GetIntKey(string(ActionListMove))
	if index.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for list move is required"))
	}

	return mo.Ok[Operator](NewListMove(index.MustGet()))
}

// MapToOperatorForListInsert converts a map to a ListInsert operator.
func (f *OperationFactory) MapToOperatorForListInsert(obj Value) mo.Result[Operator] {
	li := obj.GetKey(string(ActionListInsert))
	if li.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for list insert is required"))
	}

	if obj.HasKey(string(ActionListDelete)) {
		result := f.ValidateOperationObjectSize(obj, 3)
		if result.IsError() {
			return mo.Err[Operator](result.Error())
		}
		ld := obj.GetKey(string(ActionListDelete))
		return mo.Ok[Operator](NewListReplace(li.MustGet(), ld.MustGet()))
	}

	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	return mo.Ok[Operator](NewListInsert(li.MustGet()))
}

// MapToOperatorForListDelete converts a map to a ListDelete operator.
func (f *OperationFactory) MapToOperatorForListDelete(obj Value) mo.Result[Operator] {
	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	ld := obj.GetKey(string(ActionListDelete))
	if ld.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for list delete is required"))
	}
	return mo.Ok[Operator](NewListDelete(ld.MustGet()))
}

// MapToOperatorForListReplace converts a map to a ListReplace operator.
func (f *OperationFactory) MapToOperatorForListReplace(obj Value) mo.Result[Operator] {
	result := f.ValidateOperationObjectSize(obj, 3)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	li := obj.GetKey(string(ActionListInsert))
	ld := obj.GetKey(string(ActionListDelete))
	if li.IsAbsent() || ld.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for list replace is required"))
	}

	return mo.Ok[Operator](NewListReplace(li.MustGet(), ld.MustGet()))
}

// MapToOperatorForObjectInsert converts a map to an ObjectInsert operator.
func (f *OperationFactory) MapToOperatorForObjectInsert(obj Value) mo.Result[Operator] {
	oi := obj.GetKey(string(ActionObjectInsert))
	if obj.HasKey(string(ActionObjectDelete)) {
		result := f.ValidateOperationObjectSize(obj, 3)
		if result.IsError() {
			return mo.Err[Operator](result.Error())
		}
		od := obj.GetKey(string(ActionObjectDelete))
		return mo.Ok[Operator](NewObjectReplace(oi.MustGet(), od.MustGet()))
	}

	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	return mo.Ok[Operator](NewObjectInsert(oi.MustGet()))
}

// MapToOperatorForObjectDelete converts a map to an ObjectDelete operator.
func (f *OperationFactory) MapToOperatorForObjectDelete(obj Value) mo.Result[Operator] {
	if !obj.HasKey(string(ActionObjectDelete)) {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for object delete is required"))
	}

	result := f.ValidateOperationObjectSize(obj, 2)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	od := obj.GetKey(string(ActionObjectDelete))
	return mo.Ok[Operator](NewObjectDelete(od.MustGet()))
}

// MapToOperatorForObjectReplace converts a map to an ObjectReplace operator.
func (f *OperationFactory) MapToOperatorForObjectReplace(obj Value) mo.Result[Operator] {
	result := f.ValidateOperationObjectSize(obj, 3)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	oi := obj.GetKey(string(ActionObjectInsert))
	od := obj.GetKey(string(ActionObjectDelete))
	if oi.IsAbsent() || od.IsAbsent() {
		return mo.Err[Operator](NewError(InvalidOperation).Append("value for object replace is required"))
	}

	return mo.Ok[Operator](NewObjectReplace(oi.MustGet(), od.MustGet()))
}

// MapToOperatorForNoop converts a map to a Noop operator.
func (f *OperationFactory) MapToOperatorForNoop(obj Value) mo.Result[Operator] {
	result := f.ValidateOperationObjectSize(obj, 1)
	if result.IsError() {
		return mo.Err[Operator](result.Error())
	}

	return mo.Ok[Operator](&Noop{})
}
