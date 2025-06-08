package jsonot

// Action 表达了 JSON Object 的操作类型
type Action string

const (
	// ActionNoop 表示不执行任何操作
	ActionNoop Action = ""
	// ActionListInsert 表示在列表中插入元素
	ActionListInsert Action = "li"
	// ActionListDelete 表示在列表中删除元素
	ActionListDelete Action = "ld"
	// ActionListReplace 表示在列表中替换元素
	ActionListReplace Action = "lr"
	// ActionListMove 表示在列表中移动元素
	ActionListMove Action = "lm"
	// ActionObjectInsert 表示在对象中插入元素
	ActionObjectInsert Action = "oi"
	// ActionObjectDelete 表示在对象中删除元素
	ActionObjectDelete Action = "od"
	// ActionObjectReplace 表示在对象中替换元素
	ActionObjectReplace Action = "or"
	// ActionSubType 表示自定义子类型的操作
	ActionSubType Action = "t"
)

// SubTypeAction 定义了子类型操作的类型
type SubTypeAction string

const (
	// ActionSubTypeNumberAdd 表示在数字类型上执行加法操作
	ActionSubTypeNumberAdd SubTypeAction = "na"
	// ActionSubTypeText 表示文本子类型的操作
	ActionSubTypeText SubTypeAction = "text"
)

// SubTypeOperand 定义了子类型操作的操作数类型
const SubTypeOperand = "o"
