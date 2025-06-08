package jsonot

import (
	"fmt"
	"slices"

	"github.com/samber/mo"
)

// RouteGetOnValue 从值中获取路由
func RouteGetOnValue(val *ValueBrian, paths Path) (mo.Option[*ValueBrian], error) {
	switch val.Value.Type() {
	case Array:
		return RouteGetOnArray(val, paths)
	case Object:
		return RouteGetOnObject(val, paths)
	default:
		return mo.None[*ValueBrian](), nil
	}
}

// RouteGetOnArray 从数组上获取路径
func RouteGetOnArray(val *ValueBrian, paths Path) (mo.Option[*ValueBrian], error) {
	arr := val.Value.GetArray()
	if arr.IsError() {
		return mo.None[*ValueBrian](), arr.Error()
	}

	index := paths.FirstIndexPath()
	if index.IsAbsent() {
		return mo.None[*ValueBrian](), ErrBadPath
	}

	if index.MustGet() < 0 {
		return mo.None[*ValueBrian](), ErrBadPath
	}

	if index.MustGet() >= len(arr.MustGet()) {
		return mo.None[*ValueBrian](), nil
	}

	node := arr.MustGet()[index.MustGet()]
	if node.IsNull() {
		return mo.None[*ValueBrian](), nil
	}

	node.PackAny()
	brainNode := &ValueBrian{Value: node, Parent: val, IndexInParent: index.MustGet()}
	nextLevel := paths.NextLevel()
	if nextLevel.IsEmpty() {
		return mo.Some(brainNode), nil
	}

	nextNode, err := RouteGetOnValue(brainNode, nextLevel)
	if err != nil {
		return mo.None[*ValueBrian](), err
	}

	return nextNode, nil
}

// RouteGetOnObject 从对象上获取路径
func RouteGetOnObject(obj *ValueBrian, paths Path) (mo.Option[*ValueBrian], error) {
	key := paths.FirstKeyPath()
	if key.IsAbsent() {
		return mo.None[*ValueBrian](), ErrBadPath
	}

	node := obj.Value.GetKey(key.MustGet())
	if node.IsAbsent() {
		return mo.None[*ValueBrian](), nil
	}

	nextObj := node.MustGet()
	nextObj.PackAny()
	brainNode := &ValueBrian{Value: nextObj, Parent: obj, KeyInParent: key.MustGet()}
	nextLevel := paths.NextLevel()
	if nextLevel.IsEmpty() {
		return mo.Some(brainNode), nil
	}

	nextNode, err := RouteGetOnValue(brainNode, nextLevel)
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
		return fmt.Errorf("%w: unexpected to set value by path %s", ErrBadPath, paths.String())
	}
}

// RouteSetOnArray 在数组上设置路由
func RouteSetOnArray(arr *ValueBrian, paths Path, newValue *ValueBrian) error {
	indexOpt := paths.FirstIndexPath()
	if indexOpt.IsAbsent() {
		return fmt.Errorf("%w: apply left value for array failed with path: %s", ErrBadPath, paths.String())
	}
	index := indexOpt.MustGet()
	array := arr.Value.GetArray()
	if array.IsError() {
		return array.Error()
	}
	arrSlice := array.MustGet()
	if index < 0 || index >= len(arrSlice) {
		return fmt.Errorf("%w: index %d out of range for array with length %d", ErrBadPath, index, len(arrSlice))
	}
	if paths.Len() == 1 {
		// 直接替换该 index 的值
		arrSlice[index] = newValue.Value
		return arr.Value.Unmarshal(ValueFromArray(arrSlice).RawMessage())
	}
	// 递归设置
	child := &ValueBrian{Value: arrSlice[index], Parent: arr, IndexInParent: index}
	if err := RouteSetOnValue(child, paths.NextLevel(), newValue); err != nil {
		return err
	}
	// 更新数组
	arrSlice[index] = child.Value
	return arr.Value.Unmarshal(ValueFromArray(arrSlice).RawMessage())
}

// RouteSetOnObject 在对象上设置路由
func RouteSetOnObject(obj *ValueBrian, paths Path, newValue *ValueBrian) error {
	keyOpt := paths.FirstKeyPath()
	if keyOpt.IsAbsent() {
		return fmt.Errorf("%w: apply left value for object failed with path: %s", ErrBadPath, paths.String())
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
		leftVal, err := RouteGetOnValue(brainVal, left)
		if err != nil {
			return err
		}

		if leftVal.IsAbsent() {
			return fmt.Errorf("%w: apply left value for path %s failed", ErrBadPath, paths.String())
		}

		err = ApplyToValue(leftVal.MustGet().Value, right, operator)
		if err != nil {
			return err
		}

		log.Debugf("ready to set value on left: %s with value: %s\n", left.String(), leftVal.MustGet().Value.RawMessage())

		if err := RouteSetOnValue(brainVal, left, leftVal.MustGet()); err != nil {
			return fmt.Errorf("failed to set value on path %s: %w", left.String(), err)
		}

		log.Debugf("after apply to left: %s\n", val.RawMessage())
		return nil
	}

	val.PackAny()
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
		node := ValueFromArray(arrNode)
		log.Debugf("after apply to array: %s\n", node.RawMessage())
		// 使用 reflect 更新 val 的值
		return val.Unmarshal(node.RawMessage())

	case Object:
		log.Debugf("before apply to object: %s\n", val.RawMessage())
		node, err := ApplyToObject(val, paths, operator)
		if err != nil {
			return err
		}
		log.Debugf("after apply to object: %s\n", node.RawMessage())
		return val.Unmarshal(node.RawMessage())
	default:
		// 待实现子类型
		return fmt.Errorf(
			"%w: unsupported type for apply operator: %s from path: %s for value: %s",
			ErrBadPath, val.Type(), paths.String(), val.RawMessage(),
		)
	}
}

// ApplyToArray 将操作应用到值上
func ApplyToArray(arr []Value, paths Path, operator Operator) ([]Value, error) {
	indexOption := paths.FirstIndexPath()
	if indexOption.IsAbsent() {
		return nil, fmt.Errorf("%w: apply left value for array failed with path: %s", ErrBadPath, paths.String())
	}

	index := indexOption.MustGet()
	if index < 0 {
		return arr, nil // 如果索引不合法，直接返回原数组
	}

	switch op := operator.(type) {
	case *ListDelete:
		if op.Value.IsNumeric() || op.Value.IsString() {
			// 根据 op.value 找到索引，不使用传入的 index
			index = slices.IndexFunc(arr, func(item Value) bool { return item.Equals(op.Value) })
		}
		// 如果已经没有元素了，或者索引不合法，则直接返回
		if len(arr) == 0 {
			break
		}

		if index >= 0 && len(arr) > index {
			arr = slices.Delete(arr, index, index+1) // 删除一个元素
		}
	case *ListInsert:
		if index > len(arr) {
			arr = append(arr, op.Value)
		} else {
			arr = slices.Insert(arr, index, op.Value)
		}
	case *ListReplace:
		if op.OldValue.IsNumeric() || op.OldValue.IsString() {
			// 根据 op.value 找到索引，不使用传入的 index
			oldValueIndex := slices.IndexFunc(arr, func(item Value) bool { return item.Equals(op.OldValue) })
			if oldValueIndex != -1 {
				index = oldValueIndex
			}
		}
		// 要保证旧值存在
		if index > 0 && index < len(arr) {
			if target := arr[index]; !target.IsNull() {
				arr[index] = op.NewValue
			}
		}
	case *ListMove:
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
	}

	return arr, nil
}

// ApplyToObject 将操作应用到对象上
func ApplyToObject(obj Value, paths Path, operator Operator) (Value, error) {
	if paths.Len() < 1 {
		return nil, ErrBadPath
	}

	keyOption := paths.FirstKeyPath()
	if keyOption.IsAbsent() {
		return nil, ErrBadPath
	}

	key := keyOption.MustGet()
	switch op := operator.(type) {
	case *ObjectInsert:
		if err := obj.SetKey(key, op.Value); err != nil {
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
		if result.IsOk() && result.MustGet().IsPresent() {
			sum := result.MustGet().MustGet()
			if err := obj.SetKey(key, sum); err != nil {
				return nil, fmt.Errorf("failed to exec subtype update key %s: %w", key, err)
			}
			log.Debugf("after apply to object: %s with key: %s, value: %s\n", obj.RawMessage(), key, sum.RawMessage())
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("apply failed: unsupported operator type: %T", operator)
	}
}
