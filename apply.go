package jsonot

import (
	"fmt"
	"slices"

	"github.com/samber/mo"
)

// RouteGetOnValue 从值中获取路由
func RouteGetOnValue(val *ValueBrian, paths Path, valType ValueType) (mo.Option[*ValueBrian], error) {
	switch val.Value.Type() {
	case Array:
		return RouteGetOnArray(val, paths, valType)
	case Object:
		return RouteGetOnObject(val, paths, valType)
	default:
		return mo.None[*ValueBrian](), nil
	}
}

// RouteGetOnArray 从数组上获取路径
func RouteGetOnArray(val *ValueBrian, paths Path, valType ValueType) (mo.Option[*ValueBrian], error) {
	arr := val.Value.GetArray()
	if arr.IsError() {
		return mo.None[*ValueBrian](), arr.Error()
	}

	index := paths.FirstIndexPath()
	if index.IsAbsent() {
		return mo.None[*ValueBrian](), NewError(BadPath)
	}

	if index.MustGet() < 0 {
		return mo.None[*ValueBrian](), NewError(BadPath).Append("index must be non-negative")
	}

	brainNode := &ValueBrian{Parent: val, IndexInParent: index.MustGet()}
	if index.MustGet() >= len(arr.MustGet()) {
		brainNode.Value = NewValue(valType)
		return mo.Some(brainNode), nil
	}

	node := arr.MustGet()[index.MustGet()]
	if node.IsNull() {
		brainNode.Value = NewValue(valType)
		return mo.Some(brainNode), nil
	}

	brainNode.Value = node

	node.PackAny()
	nextLevel := paths.NextLevel()
	if nextLevel.IsEmpty() {
		return mo.Some(brainNode), nil
	}

	nextNode, err := RouteGetOnValue(brainNode, nextLevel, valType)
	if err != nil {
		return mo.None[*ValueBrian](), err
	}

	return nextNode, nil
}

// RouteGetOnObject 从对象上获取路径
func RouteGetOnObject(obj *ValueBrian, paths Path, valType ValueType) (mo.Option[*ValueBrian], error) {
	key := paths.FirstKeyPath()
	if key.IsAbsent() {
		return mo.None[*ValueBrian](), NewError(BadPath)
	}

	brainNode := &ValueBrian{Parent: obj, KeyInParent: key.MustGet()}
	node := obj.Value.GetKey(key.MustGet())
	if node.IsAbsent() {
		brainNode.Value = NewValue(valType)
		return mo.Some(brainNode), nil
	}

	brainNode.Value = node.MustGet()
	nextLevel := paths.NextLevel()
	if nextLevel.IsEmpty() {
		return mo.Some(brainNode), nil
	}

	nextNode, err := RouteGetOnValue(brainNode, nextLevel, valType)
	if err != nil {
		return mo.None[*ValueBrian](), err
	}

	return nextNode, nil
}

// RouteSetOnValue 在值上设置路由
func RouteSetOnValue(val *ValueBrian, paths Path, newValue *ValueBrian) error {
	val.Value.PackAny()
	switch val.Value.Type() {
	case Array:
		return RouteSetOnArray(val, paths, newValue)
	case Object:
		return RouteSetOnObject(val, paths, newValue)
	default:
		return NewError(BadPath).Append("raw value for update should be array or object")
	}
}

// RouteSetOnArray 在数组上设置路由
func RouteSetOnArray(arr *ValueBrian, paths Path, newValue *ValueBrian) error {
	indexOpt := paths.FirstIndexPath()
	if indexOpt.IsAbsent() {
		return NewError(BadPath).Append("index for array update is absent")
	}
	index := indexOpt.MustGet()
	array := arr.Value.GetArray()
	if array.IsError() {
		return array.Error()
	}
	arrSlice := array.MustGet()
	if index < 0 || index >= len(arrSlice) {
		return NewError(BadPath).Append("index out of bounds for array update")
	}
	if paths.Len() == 1 {
		// 直接替换该 index 的值
		arrSlice[index] = newValue.Value
		return arr.Value.UpdateArray(arrSlice)
	}

	// 递归设置
	child := &ValueBrian{Value: arrSlice[index], Parent: arr, IndexInParent: index}
	if err := RouteSetOnValue(child, paths.NextLevel(), newValue); err != nil {
		return err
	}

	// 更新数组
	arrSlice[index] = child.Value
	return arr.Value.UpdateArray(arrSlice)
}

// RouteSetOnObject 在对象上设置路由
func RouteSetOnObject(obj *ValueBrian, paths Path, newValue *ValueBrian) error {
	keyOpt := paths.FirstKeyPath()
	if keyOpt.IsAbsent() {
		return NewError(BadPath).Append("key for object update is absent")
	}
	key := keyOpt.MustGet()
	nodeOpt := obj.Value.GetKey(key)
	var node Value
	if nodeOpt.IsPresent() {
		node = nodeOpt.MustGet()
	} else {
		// 若不存在则新建空对象
		node = ValueFromAny(map[string]Value{})
	}
	if paths.Len() == 1 {
		// 直接替换该 key 的值
		return obj.Value.SetKey(key, newValue.Value)
	}
	// 递归设置
	child := &ValueBrian{Value: node, Parent: obj, KeyInParent: key}
	if err := RouteSetOnValue(child, paths.NextLevel(), newValue); err != nil {
		return err
	}
	return obj.Value.SetKey(key, child.Value)
}

// ApplyToValue 将操作应用到值上
func ApplyToValue(val Value, paths Path, operator Operator) error {
	if paths.Len() > 1 {
		left, right := paths.SplitAt(paths.Len() - 1)

		val.PackAny()
		brainVal := &ValueBrian{Value: val}
		leftVal, err := RouteGetOnValue(brainVal, left, OperatorForValueType(operator))
		if err != nil {
			return err
		}

		if leftVal.IsAbsent() {
			return NewError(InvalidParameter).Append("sub attribute or item not found")
		}

		if err := ApplyToValue(leftVal.MustGet().Value, right, operator); err != nil {
			return fmt.Errorf("failed to apply operation on value: %w", err)
		}

		if err := RouteSetOnValue(brainVal, left, leftVal.MustGet()); err != nil {
			return fmt.Errorf("failed to update applied value: %w", err)
		}

		return nil
	}

	switch val.Type() {
	case Array:
		arr := val.GetArray()
		if arr.IsError() {
			return arr.Error()
		}
		arrNode, err := ApplyToArray(arr.MustGet(), paths, operator)
		if err != nil {
			return err
		}
		return val.UpdateArray(arrNode)
	case Object:
		node, err := ApplyToObject(val, paths, operator)
		if err != nil {
			return err
		}
		return val.UpdateObject(node)
	default:
		return NewError(InvalidParameter).Append("unknown value type for apply operation: %s", val.Type())
	}
}

// ApplyToRootValue applies an operator directly on the root value.
func ApplyToRootValue(val Value, operator Operator) (Value, error) {
	switch op := operator.(type) {
	case *Noop:
		return val, nil
	case *ListInsert:
		return op.NewValue, nil
	case *ListDelete:
		return ValueFromAny(nil), nil
	case *ListReplace:
		return op.NewValue, nil
	case *ObjectInsert:
		return op.NewValue, nil
	case *ObjectDelete:
		return ValueFromAny(nil), nil
	case *ObjectReplace:
		return op.NewValue, nil
	case *SubTypeOperator:
		result := op.SubTypeFunctions.Apply(mo.Some(val), op.Value)
		if result.IsError() {
			return nil, result.Error()
		}
		if result.MustGet().IsAbsent() {
			return ValueFromAny(nil), nil
		}
		return result.MustGet().MustGet(), nil
	default:
		return nil, fmt.Errorf("apply failed: unsupported root operator type: %T", operator)
	}
}

// ApplyToArray 将操作应用到值上
func ApplyToArray(arr []Value, paths Path, operator Operator) ([]Value, error) {
	indexOption := paths.FirstIndexPath()
	if indexOption.IsAbsent() {
		return nil, NewError(BadPath).Append("index for array operation is absent")
	}

	index := indexOption.MustGet()
	if index < 0 {
		return arr, nil // 如果索引不合法，直接返回原数组
	}

	switch op := operator.(type) {
	case *ListDelete:
		arr = ApplyListDelete(arr, op, index)
	case *ListInsert:
		arr = ApplyListInsert(arr, op, index)
	case *ListReplace:
		arr = ApplyListReplace(arr, op, index)
	case *ListMove:
		arr = ApplyListMove(arr, op, index)
	}

	return arr, nil
}

// ApplyListDelete 从数组中删除元素
func ApplyListDelete(arr []Value, op *ListDelete, index int) []Value {
	// 根据 op.value 找到索引，不使用传入的 index
	index = IndexFunc(arr, op.OlvValue, index)
	// 如果已经没有元素了，或者索引不合法，则直接返回
	if len(arr) == 0 {
		return arr
	}

	if index >= 0 && len(arr) > index {
		arr = slices.Delete(arr, index, index+1) // 删除一个元素
	}

	return arr
}

// ApplyListInsert 在数组中插入元素
func ApplyListInsert(arr []Value, op *ListInsert, index int) []Value {
	if index > len(arr) {
		arr = append(arr, op.NewValue)
	} else {
		arr = slices.Insert(arr, index, op.NewValue)
	}
	return arr
}

// ApplyListReplace 在数组中替换元素
func ApplyListReplace(arr []Value, op *ListReplace, index int) []Value {
	// 根据 op.value 找到索引，不使用传入的 index
	index = IndexFunc(arr, op.OldValue, index)

	// 要保证旧值存在
	if index > 0 && index < len(arr) {
		if target := arr[index]; !target.IsNull() {
			arr[index] = op.NewValue
		}
	}

	return arr
}

// ApplyListMove 在数组中移动元素
func ApplyListMove(arr []Value, op *ListMove, index int) []Value {
	// 要判断就值是存在的
	if index > 0 && index < len(arr) {
		if index != op.NewIndex {
			oldValue := arr[index]
			arr = slices.Delete(arr, index, index+1)
			if op.NewIndex > len(arr) {
				arr = append(arr, oldValue)
			} else {
				arr = slices.Insert(arr, op.NewIndex, oldValue)
			}
		}
	}

	return arr
}

// ApplyToObject 将操作应用到对象上
func ApplyToObject(obj Value, paths Path, operator Operator) (Value, error) {
	if paths.Len() < 1 {
		return nil, NewError(BadPath).Append("path for object operation is empty")
	}

	keyOption := paths.FirstKeyPath()
	if keyOption.IsAbsent() {
		return nil, NewError(BadPath).Append("key for object operation is absent")
	}

	key := keyOption.MustGet()
	switch op := operator.(type) {
	case *ObjectInsert:
		if err := obj.SetKey(key, op.NewValue); err != nil {
			return nil, fmt.Errorf("failed to insert key %s: %w", key, err)
		}
		return obj, nil
	case *ObjectDelete:
		target := obj.GetKey(key)
		if target.IsAbsent() {
			return obj, nil // 这里就不报错了，可能是因为键不存在
		}
		if err := obj.DeleteKey(key); err != nil {
			return nil, fmt.Errorf("failed to delete key %s: %w", key, err)
		}
		return obj, nil
	case *ObjectReplace:
		if err := obj.SetKey(key, op.NewValue); err != nil {
			return nil, fmt.Errorf("failed to update object key %s: %w", key, err)
		}
		return obj, nil
	case *SubTypeOperator:
		// 处理子类型操作
		targetValue := obj.GetKey(key)
		result := op.SubTypeFunctions.Apply(targetValue, op.Value)
		if result.IsError() {
			return nil, result.Error()
		}
		if result.MustGet().IsPresent() {
			sum := result.MustGet().MustGet()
			if err := obj.SetKey(key, sum); err != nil {
				return nil, fmt.Errorf("failed to exec subtype update key %s: %w", key, err)
			}
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("apply failed: unsupported operator type: %T", operator)
	}
}

// IndexFunc is a helper function to find the index of a value in an array
func IndexFunc(arr []Value, search Value, inputIndex int) int {
	var indexes []int
	if len(arr) == 0 {
		return -1 // If the array is empty, return -1
	}

	// 先使用 inputIndex 来查找
	if inputIndex >= 0 && inputIndex < len(arr) {
		if arr[inputIndex].Equals(search) {
			return inputIndex // If the input index matches, return it
		}
	}

	if IsSimpleValue(search) {
		for k := range arr {
			if arr[k].Equals(search) {
				indexes = append(indexes, k) // Found the value, return its index
			}
		}
	}

	if len(indexes) == 1 {
		return indexes[0] // Found only one, return it
	}

	return inputIndex // If multiple found, return the input index，because we don't know which one to choose
}
