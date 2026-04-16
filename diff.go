package jsonot

import (
	"context"
	"sort"

	"github.com/samber/mo"
)

// Diff generates an operation that transforms from into to.
func (ot *JSONOperationTransformer) Diff(_ context.Context, from, to Value) mo.Result[*Operation] {
	operation := NewOperation([]*OperationComponent{})
	if err := ot.diffInto(Path{}, from, to, operation); err != nil {
		return mo.Err[*Operation](err)
	}
	return mo.Ok(operation)
}

func (ot *JSONOperationTransformer) diffInto(path Path, from, to Value, operation *Operation) error {
	if from.Equals(to) {
		return nil
	}

	switch {
	case from.IsObject() && to.IsObject():
		return ot.diffObject(path, from, to, operation)
	case from.IsArray() && to.IsArray():
		return ot.diffArray(path, from, to, operation)
	case from.IsNumeric() && to.IsNumeric():
		return ot.diffNumber(path, from, to, operation)
	default:
		return ot.appendReplace(path, from, to, operation)
	}
}

func (ot *JSONOperationTransformer) diffObject(path Path, from, to Value, operation *Operation) error {
	fromMap := from.GetMap()
	if fromMap.IsError() {
		return fromMap.Error()
	}
	toMap := to.GetMap()
	if toMap.IsError() {
		return toMap.Error()
	}

	keysSet := make(map[string]struct{}, len(fromMap.MustGet())+len(toMap.MustGet()))
	for key := range fromMap.MustGet() {
		keysSet[key] = struct{}{}
	}
	for key := range toMap.MustGet() {
		keysSet[key] = struct{}{}
	}

	keys := make([]string, 0, len(keysSet))
	for key := range keysSet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		nextPath := appendPath(path, PathElementFromKey(key))
		fromValue, inFrom := fromMap.MustGet()[key]
		toValue, inTo := toMap.MustGet()[key]

		switch {
		case inFrom && !inTo:
			component, err := NewOperationComponent(nextPath, NewObjectDelete(fromValue)).Get()
			if err != nil {
				return err
			}
			operation.Append(component)
		case !inFrom && inTo:
			component, err := NewOperationComponent(nextPath, NewObjectInsert(toValue)).Get()
			if err != nil {
				return err
			}
			operation.Append(component)
		case inFrom && inTo:
			if err := ot.diffInto(nextPath, fromValue, toValue, operation); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ot *JSONOperationTransformer) diffArray(path Path, from, to Value, operation *Operation) error {
	fromArray := from.GetArray()
	if fromArray.IsError() {
		return fromArray.Error()
	}
	toArray := to.GetArray()
	if toArray.IsError() {
		return toArray.Error()
	}

	left := fromArray.MustGet()
	right := toArray.MustGet()
	if len(left) != len(right) {
		return ot.appendReplace(path, from, to, operation)
	}

	for index := range left {
		nextPath := appendPath(path, PathElementFromIndex(index))
		if err := ot.diffInto(nextPath, left[index], right[index], operation); err != nil {
			return err
		}
	}

	return nil
}

func (ot *JSONOperationTransformer) diffNumber(path Path, from, to Value, operation *Operation) error {
	if !supportsSubtypePath(path) {
		return ot.appendReplace(path, from, to, operation)
	}

	subTypeFunctions := ot.functions.Get(ActionSubTypeNumberAdd)
	if subTypeFunctions.IsAbsent() {
		return ot.appendReplace(path, from, to, operation)
	}

	if from.IsInt() && to.IsInt() {
		delta := to.GetInt().MustGet() - from.GetInt().MustGet()
		if delta == 0 {
			return nil
		}
		operand := ValueFromPrimitive(delta)
		operand.PackAny()
		component, err := NewOperationComponent(
			path,
			NewSubTypeOperator(NewNumberAdd(), operand, subTypeFunctions.MustGet()),
		).Get()
		if err != nil {
			return err
		}
		operation.Append(component)
		return nil
	}

	delta := to.GetNumeric().MustGet() - from.GetNumeric().MustGet()
	if delta == 0 {
		return nil
	}
	operand := ValueFromPrimitive(delta)
	operand.PackAny()

	component, err := NewOperationComponent(
		path,
		NewSubTypeOperator(NewNumberAdd(), operand, subTypeFunctions.MustGet()),
	).Get()
	if err != nil {
		return err
	}
	operation.Append(component)
	return nil
}

func (ot *JSONOperationTransformer) appendReplace(path Path, from, to Value, operation *Operation) error {
	component, err := NewOperationComponent(path, replaceOperator(path, from, to)).Get()
	if err != nil {
		return err
	}
	operation.Append(component)
	return nil
}

func appendPath(path Path, element PathElement) Path {
	next := path.Clone()
	next.Paths = append(next.Paths, element)
	return next
}

func replaceOperator(path Path, from, to Value) Operator {
	if path.IsEmpty() {
		return NewObjectReplace(to, from)
	}

	last := path.Last()
	if last.IsPresent() && last.MustGet().Key != "" {
		return NewObjectReplace(to, from)
	}

	return NewListReplace(to, from)
}

func supportsSubtypePath(path Path) bool {
	if path.IsEmpty() {
		return true
	}

	last := path.Last()
	return last.IsPresent() && last.MustGet().Key != ""
}
