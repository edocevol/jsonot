package jsonot

import (
	"fmt"
)

// Operator is a type of operation that can be applied to a JSON value.
type Operator interface {
	Format(f fmt.State, verb rune)
	Action() Action
	Validates() error
	Clone() Operator
}

// OperatorForValueType returns an operator for the given value type.
func OperatorForValueType(op Operator) ValueType {
	action := op.Action()
	switch action {
	case ActionObjectInsert, ActionObjectDelete, ActionObjectReplace:
		return Object
	case ActionListInsert, ActionListDelete, ActionListMove, ActionListReplace:
		return Array
	default:
		return Null
	}
}

var (
	_ Operator = (*Noop)(nil)
	_ Operator = (*ListInsert)(nil)
	_ Operator = (*ListDelete)(nil)
	_ Operator = (*ListReplace)(nil)
	_ Operator = (*ListMove)(nil)
	_ Operator = (*ObjectInsert)(nil)
	_ Operator = (*ObjectDelete)(nil)
	_ Operator = (*ObjectReplace)(nil)
)

// Noop creates a noop operator.
type Noop struct{}

// NewNoop creates a noop operator.
func NewNoop() *Noop {
	return &Noop{}
}

// Format formats the operator for debugging.
func (n Noop) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "Noop Operator")
}

// Action returns the action of the operator.
func (n Noop) Action() Action {
	return ActionNoop
}

// Validates returns nil if the operator is valid.
func (n Noop) Validates() error {
	return nil
}

// Clone returns a copy of the operator.
func (n Noop) Clone() Operator {
	return NewNoop()
}

// ListInsert creates a list insert operator.
type ListInsert struct {
	NewValue Value
}

// Clone returns a copy of the operator.
func (l *ListInsert) Clone() Operator {
	return NewListInsert(l.NewValue)
}

// Format formats the operator for debugging.
func (l *ListInsert) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ListInsert{%v}", l.NewValue)
}

// NewListInsert creates a list insert operator.
func NewListInsert(newValue Value) *ListInsert {
	return &ListInsert{
		NewValue: newValue,
	}
}

// Action returns the action of the operator.
func (l *ListInsert) Action() Action {
	return ActionListInsert
}

// Validates returns nil if the operator is valid.
func (l *ListInsert) Validates() error {
	return nil
}

// ListDelete creates a list delete operator.
type ListDelete struct {
	OlvValue Value
}

// Clone returns a copy of the operator.
func (l *ListDelete) Clone() Operator {
	return NewListDelete(l.OlvValue)
}

// Format formats the operator for debugging.
func (l *ListDelete) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ListDelete{%v}", l.OlvValue)
}

// NewListDelete creates a list delete operator.
func NewListDelete(oldValue Value) *ListDelete {
	return &ListDelete{
		OlvValue: oldValue,
	}
}

// Action returns the action of the operator.
func (l *ListDelete) Action() Action {
	return ActionListDelete
}

// Validates returns nil if the operator is valid.
func (l *ListDelete) Validates() error {
	return nil
}

// ListReplace creates a list replace operator.
type ListReplace struct {
	NewValue Value
	OldValue Value
}

// Clone returns a copy of the operator.
func (l *ListReplace) Clone() Operator {
	return NewListReplace(l.NewValue, l.OldValue)
}

// NewListReplace creates a list replace operator.
func NewListReplace(newValue, oldValue Value) *ListReplace {
	return &ListReplace{
		NewValue: newValue,
		OldValue: oldValue,
	}
}

// Format formats the operator for debugging.
func (l *ListReplace) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ListReplace{NewValue: %v, OldValue: %v}", l.NewValue, l.OldValue)
}

// Action returns the action of the operator.
func (l *ListReplace) Action() Action {
	return ActionListReplace
}

// Validates returns nil if the operator is valid.
func (l *ListReplace) Validates() error {
	return nil
}

// ListMove creates a list move operator.
type ListMove struct {
	NewIndex int
}

// Clone returns a copy of the operator.
func (l *ListMove) Clone() Operator {
	return NewListMove(l.NewIndex)
}

// Format formats the operator for debugging.
func (l *ListMove) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ListMove{NewIndex: %d}", l.NewIndex)
}

// NewListMove creates a list move operator.
func NewListMove(newIndex int) *ListMove {
	return &ListMove{
		NewIndex: newIndex,
	}
}

// Action returns the action of the operator.
func (l *ListMove) Action() Action {
	return ActionListMove
}

// Validates returns nil if the operator is valid.
func (l *ListMove) Validates() error {
	if l.NewIndex < 0 {
		return NewError(InvalidParameter).Append("new index for list move operator must be non-negative")
	}
	return nil
}

// ObjectInsert creates an object insert operator.
type ObjectInsert struct {
	NewValue Value
}

// Clone returns a copy of the operator.
func (o *ObjectInsert) Clone() Operator {
	return NewObjectInsert(o.NewValue)
}

// Format formats the operator for debugging.
func (o *ObjectInsert) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ObjectInsert{%v}", o.NewValue)
}

// NewObjectInsert creates an object insert operator.
func NewObjectInsert(newValue Value) *ObjectInsert {
	return &ObjectInsert{
		NewValue: newValue,
	}
}

// Validates returns nil if the operator is valid.
func (o *ObjectInsert) Validates() error {
	return nil
}

// Action returns the action of the operator.
func (o *ObjectInsert) Action() Action {
	return ActionObjectInsert
}

// ObjectDelete creates an object delete operator.
type ObjectDelete struct {
	OldValue Value
}

// Clone returns a copy of the operator.
func (o *ObjectDelete) Clone() Operator {
	return NewObjectDelete(o.OldValue)
}

// NewObjectDelete creates an object delete operator.
func NewObjectDelete(value Value) *ObjectDelete {
	return &ObjectDelete{
		OldValue: value,
	}
}

// Format formats the operator for debugging.
func (o *ObjectDelete) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ObjectDelete{%v}", o.OldValue)
}

// Action returns the action of the operator.
func (o *ObjectDelete) Action() Action {
	return ActionObjectDelete
}

// Validates returns nil if the operator is valid.
func (o *ObjectDelete) Validates() error {
	return nil
}

// ObjectReplace creates an object replace operator.
type ObjectReplace struct {
	NewValue Value
	OldValue Value
}

// Clone returns a copy of the operator.
func (o *ObjectReplace) Clone() Operator {
	return NewObjectReplace(o.NewValue, o.OldValue)
}

// Format formats the operator for debugging.
func (o *ObjectReplace) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "ObjectReplace{NewValue: %v, OldValue: %v}", o.NewValue, o.OldValue)
}

// NewObjectReplace creates an object replace operator.
func NewObjectReplace(newValue, oldValue Value) *ObjectReplace {
	return &ObjectReplace{
		NewValue: newValue,
		OldValue: oldValue,
	}
}

// Action returns the action of the operator.
func (o *ObjectReplace) Action() Action {
	return ActionObjectReplace
}

// Validates returns nil if the operator is valid.
func (o *ObjectReplace) Validates() error {
	return nil
}

var _ Operator = (*SubTypeOperator)(nil)

// SubTypeOperator is an operator that has a subtype.
type SubTypeOperator struct {
	SubType SubType
	Value   Value
	// SubTypeFunctions holds the functions for the subtype operations.
	SubTypeFunctions SubTypeFunctions
}

// Clone returns a copy of the operator.
func (s *SubTypeOperator) Clone() Operator {
	return NewSubTypeOperator(s.SubType, s.Value, s.SubTypeFunctions)
}

// NewSubTypeOperator creates a new SubTypeOperator.
func NewSubTypeOperator(subType SubType, val Value, subTypeFunctions SubTypeFunctions) *SubTypeOperator {
	return &SubTypeOperator{
		SubType:          subType,
		Value:            val,
		SubTypeFunctions: subTypeFunctions,
	}
}

// Format formats the operator for debugging.
func (s *SubTypeOperator) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprintf(f, "SubTypeOperator{SubType: %s, NewValue: %v}", s.SubType.TypeName(), s.Value)
}

// Action returns the action of the operator.
func (s *SubTypeOperator) Action() Action {
	return Action(s.SubType.TypeName())
}

// Validates returns nil if the operator is valid.
func (s *SubTypeOperator) Validates() error {
	return nil
}
