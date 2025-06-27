package jsonot

import (
	"encoding/json"

	"github.com/samber/mo"
)

const (
	// NumberAddSubTypeName is the name of the number add subtype
	NumberAddSubTypeName = "na"
	// TextSubTypeName is the name of the text subtype
	TextSubTypeName = "text"
)

var (
	_ SubTypeFunctions = (*NumberAddSubType)(nil)
	_ SubTypeFunctions = (*TextSubType)(nil)
)

// SubTypeFunctions 定义非 json 操作的子类型操作
type SubTypeFunctions interface {
	OperatorName() string
	Invert(subType SubType, path Path, subTypeOperand Value) mo.Result[Operator]
	Merge(baseOperand Value, other Operator) mo.Option[Operator]
	Transform(newVal, baseVal Value, side TransformSide) mo.Result[[]Value]
	Apply(val mo.Option[Value], subTypeOperand Value) mo.Result[mo.Option[Value]]
	ValidateOperand(val Value) error
}

var (
	_ SubType = (*NumberAdd)(nil)
	_ SubType = (*Text)(nil)
	_ SubType = (*Custom)(nil)
)

// SubType 定义非 json 操作的子类型
type SubType interface {
	// TypeName 返回子类型的名称
	TypeName() string
}

// NumberAdd 是一个数字相加的子类型
type NumberAdd struct {
}

// NewNumberAdd 创建一个数字相加的子类型
func NewNumberAdd() *NumberAdd {
	return &NumberAdd{}
}

// TypeName 返回子类型的名称
func (*NumberAdd) TypeName() string {
	return NumberAddSubTypeName
}

// Text 是一个文本操作的子类型
type Text struct {
}

// NewText 创建一个文本操作的子类型
func NewText() *Text {
	return &Text{}
}

// TypeName 返回子类型的名称
func (Text) TypeName() string {
	return TextSubTypeName
}

// Custom 是一个自定义操作的子类型
type Custom struct {
	Name string
}

// NewCustom 创建一个自定义操作的子类型
func NewCustom(name string) *Custom {
	return &Custom{Name: name}
}

// TypeName 返回子类型的名称
func (c Custom) TypeName() string {
	return c.Name
}

// SubTypeTryFromValue 将 Value 转换为 SubType
func SubTypeTryFromValue(value Value) mo.Result[SubType] {
	if value.IsString() {
		subType := value.GetString().MustGet()
		switch subType {
		case NumberAddSubTypeName:
			return mo.Ok[SubType](NewNumberAdd())
		case TextSubTypeName:
			return mo.Ok[SubType](NewText())
		}
		return mo.Ok[SubType](NewCustom(subType))
	}

	return mo.Err[SubType](NewError(InvalidOperation).Append("not recognized subtype type: %s", value.Type()))
}

// SubTypeFunctionsHolder 定义了子类型操作的持有者接口
type SubTypeFunctionsHolder interface {
	Register(subType SubTypeAction, fn SubTypeFunctions)
	Unregister(subType SubTypeAction)
	Get(subType SubTypeAction) mo.Option[SubTypeFunctions]
}

// SubTypeFunctionsHolderImpl 子类型操作的持有者
type SubTypeFunctionsHolderImpl struct {
	subtypeOperators map[SubTypeAction]SubTypeFunctions
}

// NewSubTypeFunctionsHolder 创建一个子类型操作的持有者
func NewSubTypeFunctionsHolder() *SubTypeFunctionsHolderImpl {
	sfh := &SubTypeFunctionsHolderImpl{
		subtypeOperators: make(map[SubTypeAction]SubTypeFunctions),
	}
	sfh.subtypeOperators[NumberAddSubTypeName] = NewNumberAddSubType()
	sfh.subtypeOperators[TextSubTypeName] = NewTextSubType()
	return sfh
}

// Register 注册一个子类型操作
func (sfh *SubTypeFunctionsHolderImpl) Register(subType SubTypeAction, fn SubTypeFunctions) {
	if subType == NumberAddSubTypeName || subType == TextSubTypeName {
		return
	}

	sfh.subtypeOperators[subType] = fn
}

// Unregister 注销一个子类型操作
func (sfh *SubTypeFunctionsHolderImpl) Unregister(subType SubTypeAction) {
	if subType == NumberAddSubTypeName || subType == TextSubTypeName {
		return
	}

	delete(sfh.subtypeOperators, subType)
}

// Get 获取一个子类型操作
func (sfh *SubTypeFunctionsHolderImpl) Get(subType SubTypeAction) mo.Option[SubTypeFunctions] {
	if fn, exists := sfh.subtypeOperators[subType]; exists {
		return mo.Some[SubTypeFunctions](fn)
	}
	return mo.None[SubTypeFunctions]()
}

var _ SubTypeFunctions = (*NumberAddSubType)(nil)

// NumberAddSubType 返回数字相加的子类型操作
type NumberAddSubType struct {
}

// NewNumberAddSubType 创建一个数字相加的子类型操作
func NewNumberAddSubType() *NumberAddSubType {
	return &NumberAddSubType{}
}

// OperatorName 实现 SubTypeFunctions 接口的 OperatorName 方法
func (n *NumberAddSubType) OperatorName() string {
	return NumberAddSubTypeName
}

// Invert 实现 SubTypeFunctions 接口的 Invert 方法
func (n *NumberAddSubType) Invert(subType SubType, _ Path, subTypeOperand Value) mo.Result[Operator] {
	if err := n.ValidateOperand(subTypeOperand); err != nil {
		return mo.Err[Operator](err)
	}

	if subTypeOperand.IsInt() {
		// 如果是整数类型，直接取反
		val := subTypeOperand.GetInt().MustGet()
		invertVal := ValueFromPrimitive(-val)
		return mo.Ok[Operator](NewSubTypeOperator(subType, invertVal, n))
	}

	val := subTypeOperand.GetNumeric().MustGet()
	invertVal := ValueFromPrimitive(-val)

	return mo.Ok[Operator](NewSubTypeOperator(subType, invertVal, n))
}

// Merge 实现 SubTypeFunctions 接口的 Merge 方法
func (n *NumberAddSubType) Merge(baseOperand Value, other Operator) mo.Option[Operator] {
	subType, ok := other.(*SubTypeOperator)
	if !ok {
		return mo.None[Operator]()
	}
	// 判断 baseOperand 和 subType.NewValue 的类型是否匹配
	if baseOperand.IsInt() && subType.Value.IsInt() {
		// 如果都是整数类型，直接相加
		newVal := ValueFromPrimitive(baseOperand.GetInt().MustGet() + subType.Value.GetInt().MustGet())
		return mo.Some[Operator](NewSubTypeOperator(NewNumberAdd(), newVal, n))
	}

	if baseOperand.IsNumeric() && subType.Value.IsNumeric() {
		// 如果都是数字类型，直接相加
		newVal := ValueFromPrimitive(baseOperand.GetNumeric().MustGet() + subType.Value.GetNumeric().MustGet())
		return mo.Some[Operator](NewSubTypeOperator(NewNumberAdd(), newVal, n))
	}

	return mo.None[Operator]()
}

// Transform 实现 SubTypeFunctions 接口的 Transform 方法
//
// 该方法用于转换新值和基础值，返回一个节点数组
func (n *NumberAddSubType) Transform(newVal, _ Value, _ TransformSide) mo.Result[[]Value] {
	return mo.Ok[[]Value]([]Value{newVal})
}

// Apply 实现 SubTypeFunctions 接口的 Apply 方法
func (n *NumberAddSubType) Apply(
	val mo.Option[Value], subTypeOperand Value,
) mo.Result[mo.Option[Value]] {
	if err := n.ValidateOperand(subTypeOperand); err != nil {
		return mo.Err[mo.Option[Value]](err)
	}

	if val.IsAbsent() || val.MustGet().IsNull() {
		addVal := subTypeOperand.GetNumeric()
		if addVal.IsOk() {
			return mo.Ok[mo.Option[Value]](mo.Some(ValueFromPrimitive(addVal.MustGet())))
		}
		return mo.Ok[mo.Option[Value]](mo.Some(ValueFromPrimitive(0)))
	}

	oldVal := val.MustGet()
	if oldVal.IsInt() && subTypeOperand.IsInt() {
		sum := oldVal.GetInt().MustGet() + subTypeOperand.GetInt().MustGet()
		return mo.Ok[mo.Option[Value]](mo.Some(ValueFromPrimitive(sum)))
	}

	sum := oldVal.GetNumeric().MustGet() + subTypeOperand.GetNumeric().MustGet()
	return mo.Ok[mo.Option[Value]](mo.Some(ValueFromPrimitive(sum)))
}

// ValidateOperand 实现 SubTypeFunctions 接口的 ValidateOperand 方法
func (n *NumberAddSubType) ValidateOperand(val Value) error {
	if !val.IsNumeric() {
		return NewError(InvalidOperation).Append("operand for na is not a number")
	}

	return nil
}

const (
	// TextOperandOffsetKey 是文本操作偏移量的键名
	TextOperandOffsetKey = "p"
	// TextOperandInsertKey 是文本操作插入值的键名
	TextOperandInsertKey = "i"
	// TextOperandDeleteKey 是文本操作删除值的键名
	TextOperandDeleteKey = "d"
)

// TextOperand 是一个文本操作的子类型操作
type TextOperand struct {
	InsertVal mo.Option[string] `json:"i,omitempty"`
	DeleteVal mo.Option[string] `json:"d,omitempty"`
	Offset    int               `json:"p,omitempty"`
}

// NewInsertTextOperand 创建一个文本插入操作的子类型操作
func NewInsertTextOperand(offset int, insertVal string) *TextOperand {
	return &TextOperand{
		Offset:    offset,
		InsertVal: mo.Some(insertVal),
		DeleteVal: mo.None[string](),
	}
}

// NewDeleteTextOperand 创建一个文本删除操作的子类型操作
func NewDeleteTextOperand(offset int, deleteVal string) *TextOperand {
	return &TextOperand{
		Offset:    offset,
		InsertVal: mo.None[string](),
		DeleteVal: mo.Some(deleteVal),
	}
}

// InvertObject 将传入的 TextOperand 反转为一个新的 TextOperand
func (t *TextOperand) InvertObject() mo.Result[*TextOperand] {
	if t.GetInsertVal().IsPresent() {
		return mo.Ok[*TextOperand](NewDeleteTextOperand(t.Offset, t.GetInsertVal().MustGet()))
	}

	if t.GetDeleteVal().IsPresent() {
		return mo.Ok[*TextOperand](NewInsertTextOperand(t.Offset, t.GetDeleteVal().MustGet()))
	}

	return mo.Err[*TextOperand](NewError(InvalidOperation).Append("text operand no value to invert"))
}

// TransformPosition 返回一个新的偏移量
func (t *TextOperand) TransformPosition(pos int, insertAfter bool) int {
	p := t.Offset
	insertVal := t.GetInsertVal()
	if insertVal.IsPresent() {
		if p < pos || (p == pos && insertAfter) {
			// 如果当前偏移量小于目标偏移量，或者等于目标偏移量并且是在其后插入
			return p + len(insertVal.MustGet())
		}
		return pos
	} else if pos <= p {
		return pos
	} else if pos <= p+len(t.MustGetDeleteVal()) {
		// 如果当前偏移量小于等于目标偏移量，并且目标偏移量在删除的范围内
		return p
	}

	// 否则，返回目标偏移量
	return pos - len(t.MustGetDeleteVal())
}

// IsInsert 返回 true 如果是插入操作
func (t *TextOperand) IsInsert() bool {
	return t.InsertVal.IsPresent()
}

// IsDelete 返回 true 如果是删除操作
func (t *TextOperand) IsDelete() bool {
	return t.DeleteVal.IsPresent()
}

// GetOffset 返回操作的偏移量
func (t *TextOperand) GetOffset() int {
	return t.Offset
}

// GetInsertVal 返回插入的值，如果是插入操作
func (t *TextOperand) GetInsertVal() mo.Option[string] {
	if t.IsInsert() {
		return t.InsertVal
	}
	return mo.None[string]()
}

// GetDeleteVal 返回删除的值，如果是删除操作
func (t *TextOperand) GetDeleteVal() mo.Option[string] {
	if t.IsDelete() {
		return t.DeleteVal
	}
	return mo.None[string]()
}

// MustGetInsertVal 返回插入的值，如果是插入操作，否则会 panic
func (t *TextOperand) MustGetInsertVal() string {
	return t.InsertVal.OrEmpty()
}

// MustGetDeleteVal 返回删除的值，如果是删除操作，否则会 panic
func (t *TextOperand) MustGetDeleteVal() string {
	return t.DeleteVal.OrEmpty()
}

// ToValue 将 TextOperand 转换为节点
func (t *TextOperand) ToValue() Value {
	m := map[string]any{}
	m[TextOperandOffsetKey] = t.Offset
	if t.InsertVal.IsPresent() {
		m[TextOperandInsertKey] = t.InsertVal.MustGet()
	}
	if t.DeleteVal.IsPresent() {
		m[TextOperandDeleteKey] = t.DeleteVal.MustGet()
	}

	bytes, _ := json.Marshal(m)
	node, _ := UnmarshalValue(bytes)

	return node
}

// FromValue 将节点转换为 TextOperand
func (t *TextOperand) FromValue(node Value) error {
	p := node.GetIntKey(TextOperandOffsetKey)
	if p.IsAbsent() {
		return NewError(InvalidOperation).Append("text operand must define an offset")
	}

	t.Offset = p.MustGet()
	insertNode := node.GetKey(TextOperandInsertKey)
	if insertNode.IsPresent() { // 如果存在插入的值
		t.InsertVal = mo.Some(insertNode.MustGet().GetString().OrEmpty())
	}

	deleteNode := node.GetKey(TextOperandDeleteKey)
	if deleteNode.IsPresent() { // 如果同时存在删除的值
		t.DeleteVal = mo.Some(deleteNode.MustGet().GetString().OrEmpty())
	}

	return nil
}

var _ SubTypeFunctions = (*TextSubType)(nil)

// TextSubType 是一个文本操作的子类型操作
type TextSubType struct {
}

// NewTextSubType 创建一个文本操作的子类型操作
func NewTextSubType() *TextSubType {
	return &TextSubType{}
}

// OperatorName 实现 SubTypeFunctions 接口的 OperatorName 方法
func (t *TextSubType) OperatorName() string {
	return TextSubTypeName
}

// Invert 实现 SubTypeFunctions 接口的 Invert 方法
//
// 该方法用于反转文本操作的子类型操作
func (t *TextSubType) Invert(subType SubType, _ Path, subTypeOperand Value) mo.Result[Operator] {
	var textOperand TextOperand
	if err := textOperand.FromValue(subTypeOperand); err != nil {
		return mo.Err[Operator](err)
	}

	invertedOperand := textOperand.InvertObject()
	if invertedOperand.IsError() {
		return mo.Err[Operator](invertedOperand.Error())
	}

	node := invertedOperand.MustGet().ToValue()
	return mo.Ok[Operator](NewSubTypeOperator(subType, node, t))
}

// Merge 实现 SubTypeFunctions 接口的 Merge 方法
//
// 该方法用于合并两个文本操作的子类型操作
func (t *TextSubType) Merge(baseOperand Value, other Operator) mo.Option[Operator] {
	subType, ok := other.(*SubTypeOperator)
	if !ok {
		return mo.None[Operator]()
	}

	// 判断 baseOperand 和 subType.NewValue 的类型是否匹配
	baseTextOperant := &TextOperand{}
	otherTextOperant := &TextOperand{}

	if err := baseTextOperant.FromValue(baseOperand); err != nil {
		return mo.None[Operator]()
	}
	if err := otherTextOperant.FromValue(subType.Value); err != nil {
		return mo.None[Operator]()
	}

	if baseTextOperant.IsInsert() &&
		otherTextOperant.IsInsert() &&
		baseTextOperant.Offset <= otherTextOperant.Offset &&
		otherTextOperant.Offset <= baseTextOperant.Offset+len(baseTextOperant.MustGetInsertVal()) {

		baseInsertVal := baseTextOperant.MustGetInsertVal()
		otherInsertVal := otherTextOperant.MustGetInsertVal()

		// 需要考虑这里的切片溢出问题
		splitAt := otherTextOperant.Offset - baseTextOperant.Offset
		left := SubString(baseInsertVal, 0, splitAt)
		right := SubString(baseInsertVal, splitAt, len(baseInsertVal))

		node := NewInsertTextOperand(baseTextOperant.Offset, left+otherInsertVal+right).ToValue()
		return mo.Some[Operator](NewSubTypeOperator(NewText(), node, t))
	}

	if baseTextOperant.IsDelete() &&
		otherTextOperant.IsDelete() &&
		otherTextOperant.Offset <= baseTextOperant.Offset &&
		baseTextOperant.Offset <= otherTextOperant.Offset+len(otherTextOperant.MustGetDeleteVal()) {
		baseDeleteVal := baseTextOperant.MustGetDeleteVal()
		otherDeleteVal := otherTextOperant.MustGetDeleteVal()

		// 需要考虑这里的切片溢出问题
		splitAt := baseTextOperant.Offset - otherTextOperant.Offset
		left := SubString(otherDeleteVal, 0, splitAt)
		right := SubString(otherDeleteVal, splitAt, len(otherDeleteVal))

		node := NewDeleteTextOperand(otherTextOperant.Offset, left+baseDeleteVal+right).ToValue()
		return mo.Some[Operator](NewSubTypeOperator(NewText(), node, t))
	}

	return mo.None[Operator]()
}

// Transform 实现 SubTypeFunctions 接口的 Transform 方法
//
// 该方法用于转换新值和基础值，返回一个节点数组
func (t *TextSubType) Transform(newVal, baseVal Value, side TransformSide) mo.Result[[]Value] {
	newTextOperand := &TextOperand{}
	baseTextOperand := &TextOperand{}

	if err := newTextOperand.FromValue(newVal); err != nil {
		return mo.Err[[]Value](err)
	}
	if err := baseTextOperand.FromValue(baseVal); err != nil {
		return mo.Err[[]Value](err)
	}

	var result []Value

	if newTextOperand.IsInsert() {
		p := baseTextOperand.TransformPosition(newTextOperand.Offset, side == TransformSideRight)
		result = append(result, NewInsertTextOperand(p, newTextOperand.MustGetInsertVal()).ToValue())
		return mo.Ok[[]Value](result)
	}

	deleteStr := newTextOperand.MustGetDeleteVal()
	baseInsertVal := baseTextOperand.GetInsertVal()
	if baseInsertVal.IsPresent() {
		baseP := baseTextOperand.Offset
		newP := newTextOperand.Offset
		if newTextOperand.Offset < baseTextOperand.Offset {
			trimmedStr := SubString(deleteStr, 0, baseP-newP)
			result = append(result, NewDeleteTextOperand(newP, trimmedStr).ToValue())
		}
		deleteStr = SubString(deleteStr, baseP-newP, len(deleteStr))
		if deleteStr != "" {
			remainOffset := newTextOperand.Offset + len(baseInsertVal.MustGet())
			result = append(result, NewDeleteTextOperand(remainOffset, deleteStr).ToValue())
		}
		return mo.Ok[[]Value](result)
	}

	// 如果没有插入操作，直接删除
	baseDeleteStr := newTextOperand.MustGetDeleteVal()
	if newTextOperand.Offset >= baseTextOperand.Offset+len(baseDeleteStr) {
		offset := newTextOperand.Offset - len(baseDeleteStr)
		result = append(result, NewDeleteTextOperand(offset, deleteStr).ToValue())
	} else if newTextOperand.Offset+len(deleteStr) <= baseTextOperand.Offset {
		result = append(result, NewDeleteTextOperand(newTextOperand.Offset, deleteStr).ToValue())
	} else {
		newDeleteStr := ""
		if newTextOperand.Offset < baseTextOperand.Offset {
			newDeleteStr = SubString(deleteStr, 0, baseTextOperand.Offset-newTextOperand.Offset)
		}
		if newTextOperand.Offset+len(deleteStr) > baseTextOperand.Offset+len(baseDeleteStr) {
			newDeleteStr += SubString(deleteStr,
				baseTextOperand.Offset+len(baseDeleteStr)-newTextOperand.Offset, len(deleteStr))
		}

		if newDeleteStr != "" {
			offset := newTextOperand.TransformPosition(newTextOperand.Offset, false)
			result = append(result, NewDeleteTextOperand(offset, newDeleteStr).ToValue())
		}
	}
	return mo.Ok[[]Value](result)
}

// Apply 实现 SubTypeFunctions 接口的 Apply 方法
//
// 该方法用于应用文本操作的子类型操作
func (t *TextSubType) Apply(val mo.Option[Value], subTypeOperand Value) mo.Result[mo.Option[Value]] {
	if err := t.ValidateOperand(subTypeOperand); err != nil {
		return mo.Err[mo.Option[Value]](err)
	}

	subTypeOperandText := &TextOperand{}
	if err := subTypeOperandText.FromValue(subTypeOperand); err != nil {
		return mo.Err[mo.Option[Value]](err)
	}

	if val.IsAbsent() || val.MustGet().IsNull() {
		insert := subTypeOperandText.GetInsertVal()
		if insert.IsPresent() {
			return mo.Ok(mo.Some(ValueFromPrimitive(insert.OrEmpty())))
		}
		return mo.Ok(mo.None[Value]())
	}

	p := subTypeOperandText.Offset
	v := val.MustGet()
	if v.IsNull() {
		return mo.Ok(mo.Some(ValueFromPrimitive("")))
	}
	s, err := v.GetString().Get()
	if err != nil {
		return mo.Err[mo.Option[Value]](NewError(InvalidParameter).Append("text operation value not string"))
	}

	insertVal := subTypeOperandText.GetInsertVal()
	if insertVal.IsPresent() {
		if p <= len(s) {
			// 插入操作
			newStr := SubString(s, 0, p) + insertVal.MustGet() + SubString(s, p, len(s))
			return mo.Ok(mo.Some(ValueFromPrimitive(newStr)))
		}
		// 如果偏移量大于字符串长度，直接追加
		return mo.Ok(mo.Some(ValueFromPrimitive(s + insertVal.MustGet())))
	}

	toDelete := subTypeOperandText.MustGetDeleteVal()
	deletedStr := SubString(s, p, len(toDelete))
	if toDelete != deletedStr {
		return mo.Err[mo.Option[Value]](
			NewError(InvalidOperation).Append("text operation delete value does not match the string "),
		)
	}

	if p < len(s) {
		// 删除操作
		newStr := SubString(s, 0, p) + SubString(s, p+len(toDelete), len(s))
		return mo.Ok(mo.Some(ValueFromPrimitive(newStr)))
	}

	return mo.Ok(mo.Some(ValueFromPrimitive(s))) // 如果偏移量大于字符串长度，直接返回原字符串
}

// ValidateOperand 实现 SubTypeFunctions 接口的 ValidateOperand 方法
//
// 该方法用于验证操作数是否有效
func (t *TextSubType) ValidateOperand(val Value) error {
	offset := val.GetIntKey(TextOperandOffsetKey)
	if offset.IsAbsent() {
		return NewError(InvalidOperation).Append("text operation value must be absent")
	}

	insert := val.GetKey(TextOperandInsertKey)
	if insert.IsPresent() && !insert.MustGet().IsString() {
		return NewError(InvalidOperation).Append("text operation value must be a string")
	}

	deleteVal := val.GetKey(TextOperandDeleteKey)
	if deleteVal.IsPresent() && !deleteVal.MustGet().IsString() {
		return NewError(InvalidOperation).Append("text operation value must be a string")
	}

	return nil
}

// SubString 返回一个子字符串操作的子类型操作
func SubString(s string, start, end int) string {
	if start < 0 || end < 0 {
		return ""
	}

	if start >= len(s) {
		return ""
	}

	if end > len(s) {
		end = len(s)
	}

	return s[start:end]
}
