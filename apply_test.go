package jsonot

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestApplyNumberAdd 测试 ApplyNumberAdd 操作
func TestApplyNumberAdd(t *testing.T) {
	t.Run("TestApplyNumberAddCase1", func(t *testing.T) {
		line := `
{"p1": 10}
[{"p":["p1"], "na":100}]
{"p1":110}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyNumberAddCase2", func(t *testing.T) {
		line := `
{"p1": 10}
[{"p":["p1"], "na":-100}]
{"p1":-90}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyNumberAddCase3", func(t *testing.T) {
		line := `
{"p1": 0.1}
[{"p":["p1"], "na":-0.1}]
{"p1":0.0}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 3 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyNumberAddCase4", func(t *testing.T) {
		line := `
{"p1": 10}
[{"p":["p1"], "t": "na", "o":100}]
{"p1":110}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 4 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyNumberAddCase5", func(t *testing.T) {
		line := `
{"p1": 10}
[{"p":["p1"], "t": "na", "o":-100}]
{"p1":-90}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 5 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyNumberAddCase6", func(t *testing.T) {
		line := `
{"p1": 0.1}
[{"p":["p1"], "t": "na", "o":-0.1}]
{"p1":0.0}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyNumberAdd case 6 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyText 测试 ApplyText 操作
func TestApplyText(t *testing.T) {
	t.Run("TestApplyTextCase1", func(t *testing.T) {
		line := `
{"p1": null}
[{"p":["p1"], "t": "text", "o": {"p":2, "i":"hello"}}]
{"p1":"hello"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase2", func(t *testing.T) {
		line := `
{"p1": null}
[{"p":["p1"], "t": "text", "o": {"p":2, "d":"hello"}}]
{"p1":null}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase3", func(t *testing.T) {
		line := `
{}
[{"p":["p1"], "t": "text", "o": {"p":2, "i":"hello"}}]
{"p1":"hello"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 3 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase4", func(t *testing.T) {
		line := `
{}
[{"p":["p1"], "t": "text", "o": {"p":2, "d":"hello"}}]
{}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 4 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase5", func(t *testing.T) {
		line := `
{"p1": "Mr. J"}
[{"p":["p1"], "t": "text", "o": {"p":5, "i":", hello"}}]
{"p1": "Mr. J, hello"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 5 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase6", func(t *testing.T) {
		line := `
{"p1": "Mr. J"}
[{"p":["p1"], "t": "text", "o": {"p":0, "i":"hello, "}}]
{"p1": "hello, Mr. J"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 6 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyTextCase7", func(t *testing.T) {
		line := `
{"p1": "AB"}
[{"p":["p1"], "t": "text", "o": {"p":1, "i":" Middle "}}]
{"p1": "A Middle B"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyText case 7 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyObjectInsert 测试 TestApplyObjectInsert 操作
func TestApplyObjectInsert(t *testing.T) {
	t.Run("TestApplyObjectCase1", func(t *testing.T) {
		line := `
{}
[{"p":["p1"], "oi":{"p2":{}}}]
{"p1":{"p2":{}}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase2", func(t *testing.T) {
		line := `
{}
[{"p":["p1"], "oi":200}]
{"p1":200}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase3", func(t *testing.T) {
		line := `
{"x":"a"}
[{"p":["y"],"oi":"b"}]
{"x":"a","y":"b"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 3 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase4", func(t *testing.T) {
		line := `
{"p1":{"p2":{}}}
[{"p":["p1", "p2"], "oi":{"p3":[1, {"p4":{}}]}}]
{"p1":{"p2":{"p3":[1,{"p4":{}}]}}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 4 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase5", func(t *testing.T) {
		line := `
{"p1":{"p2":{"p3":[1,{"p4":{}}]}}}
[{"p":["p1", "p2", "p3", 1, "p4"], "oi":{"p5":[1, 2]}}]
{"p1":{"p2":{"p3":[1,{"p4":{"p5":[1,2]}}]}}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 5 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase6", func(t *testing.T) {
		line := `
{"p1":{"p2":{"p3":[1,{"p4":{"p5":[1,2]}}]}}}
[{"p":["p1", "p2", "p3", 1, "p4"], "oi":[3,4]}]
{"p1":{"p2":{"p3":[1,{"p4":[3,4]}]}}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 6 failed, expected %s, got %s", expected, actual)
	})

	t.Run("TestApplyObjectCase7", func(t *testing.T) {
		line := `
{}
[{"p":["p1"], "oi":"v2"}]
{"p1":"v2"}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObject case 7 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyObjectDelete 测试 TestApplyObjectDelete 操作
func TestApplyObjectDelete(t *testing.T) {
	t.Run("delete to deep inner object with number index in path", func(t *testing.T) {
		line := `
{"p1":{"p2":{"p3":[1,{"level41":[1,2], "level42":[3,4]}]}}}
[{"p":["p1", "p2", "p3", 1, "level41"], "od":[1, 2]}]
{"p1":{"p2":{"p3":[1,{"level42":[3,4]}]}}}
`

		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObjectDelete case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("delete to inner object", func(t *testing.T) {
		line := `	
{"p1":{"p2":{"p3":[1,{"level42":[3,4]}]}}}
[{"p":["p1", "p2", "p3"], "od":[1,{"level41":[1,2], "level42":[3,4]}]}]
{"p1":{"p2":{}}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObjectDelete case 2 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyObjectReplace 测试 TestApplyObjectReplace 操作
func TestApplyObjectReplace(t *testing.T) {
	t.Run("replace deep inner object with number index in path", func(t *testing.T) {
		line := `
{"p1":{"p2":{"p3":[1,{"level41":[1,2], "level42":[3,4]}]}}}
[{"p":["p1", "p2", "p3", 1, "level41"], "oi":{"5":"6"}, "od":[1, 2]}]
{"p1":{"p2":{"p3":[1,{"level41":{"5":"6"},"level42":[3,4]}]}}}

`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObjectReplace case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("replace to inner object", func(t *testing.T) {
		line := `
{"p1":{"p2":{"p3":[1,{"level41":{"5":"6"},"level42":[3,4]}]}}}
[{"p":["p1", "p2"], "oi":"hello", "od":{"p3":[1,{"level41":[1,2], "level42":[3,4]}]}}]
{"p1":{"p2":"hello"}}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyObjectReplace case 2 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyListInsert 测试 TestApplyListInsert 操作
func TestApplyListInsert(t *testing.T) {
	t.Run("insert to empty array", func(t *testing.T) {
		line := `
{"p1": []}
[{"p":["p1", 0], "li":{"hello":[1]}}]
{"p1":[{"hello":[1]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListInsert case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("insert to array", func(t *testing.T) {
		line := `
{"p1":[{"hello":[1]}]}
[{"p":["p1", 0], "li":1}]
{"p1":[1,{"hello":[1]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListInsert case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("insert to inner array", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1]}]}
[{"p":["p1", 1, "hello",1], "li":[7,8]}]
{"p1":[1,{"hello":[1,[7,8]]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListInsert case 3 failed, expected %s, got %s", expected, actual)
	})

	t.Run("append", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8]]}]}
[{"p":["p1", 10], "li":[2,3]}]
{"p1":[1,{"hello":[1,[7,8]]},[2,3]]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListInsert case 4 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyListDelete 测试 TestApplyListDelete 操作
func TestApplyListDelete(t *testing.T) {
	t.Run("delete empty array", func(t *testing.T) {
		line := `
{"p1": []}
[{"p":["p1", 0], "ld":{}}]
{"p1":[]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListDelete case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("delete inner array", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8]]}]}
[{"p":["p1", 1, "hello", 1], "ld":[7,8]}]
{"p1":[1,{"hello":[1]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListDelete case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("delete inner object", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1]}]}
[{"p":["p1", 1], "ld":{"hello":[1,[7,8]]}}]
{"p1":[1]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListDelete case 3 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyListReplace 测试 TestApplyListReplace 操作
func TestApplyListReplace(t *testing.T) {
	t.Run("replace from inner array", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8]]}]}
[{"p":["p1", 1, "hello", 1], "li":{"hello":"world"}, "ld":[7,8]}]
{"p1":[1,{"hello":[1,{"hello":"world"}]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListReplace case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("replace from inner object", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,{"hello":"world"}]}]}
[{"p":["p1", 1], "li": {"hello":"world"}, "ld":{"hello":[1,[7,8]]}}]
{"p1":[1,{"hello":"world"}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListReplace case 2 failed, expected %s, got %s", expected, actual)
	})
}

// TestApplyListMove 测试 TestApplyListMove 操作
func TestApplyListMove(t *testing.T) {
	t.Run("move left", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8], 9, 10]}]}
[{"p":["p1", 1, "hello", 2], "lm":1}]
{"p1":[1,{"hello":[1,9,[7,8],10]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListMove case 1 failed, expected %s, got %s", expected, actual)
	})

	t.Run("move right", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8], 9, 10]}]}
[{"p":["p1", 1, "hello", 1], "lm":2}]
{"p1":[1,{"hello":[1,9,[7,8],10]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListMove case 2 failed, expected %s, got %s", expected, actual)
	})

	t.Run("stay", func(t *testing.T) {
		line := `
{"p1":[1,{"hello":[1,[7,8], 9, 10]}]}
[{"p":["p1", 1, "hello", 1], "lm":1}]
{"p1":[1,{"hello":[1,[7,8],9,10]}]}
`
		ot := NewJSONOperationTransformer()
		actual, expected := RunApplyTestCase(t, ot, line)
		assert.JSONEqf(t, actual, expected, "ApplyListMove case 3 failed, expected %s, got %s", expected, actual)
	})
}

// RunApplyTestCase 执行应用测试用例
func RunApplyTestCase(t *testing.T, ot *JSONOperationTransformer, lines string) (actual, expected string) {
	actualVal, expectedVal, op := ParseApplyCase(t, ot, lines)
	result := ot.Apply(context.Background(), actualVal, op)
	if result.IsError() {
		t.Errorf("failed to apply operation: %v", result.Error())
		return string(actualVal.RawMessage()), ""
	}

	return string(result.MustGet().RawMessage()), string(expectedVal.RawMessage())
}

// ParseApplyCase 解析应用测试用例
// 第一行是初始值，后续行是操作，最后一行是预期结果
//
// {"p1": 10}
// [{"p":["p1"], "na":100}]
// {"p1":110}
func ParseApplyCase(t *testing.T, ot *JSONOperationTransformer, lines string) (value, expected Value, op *Operation) {
	// 去除空行
	lines = strings.TrimSpace(lines)
	cases := strings.Split(lines, "\n")
	if len(cases) != 3 {
		t.Fatalf("failed to parse apply case, expected 3 lines, got %d", len(cases))
	}

	var err error
	value, err = UnmarshalValue([]byte(cases[0]))
	if err != nil {
		t.Fatalf("failed to parse initial value: %v", err)
	}

	var opNode Value
	opNode, err = UnmarshalValue([]byte(cases[1]))
	if err != nil {
		t.Fatalf("failed to parse operation node: %v", err)
	}
	log.Debugf("got operation node: %s\n", opNode.RawMessage())
	op = NewOperation(ot.OperationComponentsFromNode(opNode).MustGet())

	expected, err = UnmarshalValue([]byte(cases[2]))
	if err != nil {
		t.Fatalf("failed to parse expected value: %v", err)
	}

	log.Debugf("Parsed apply case: value=%s, expected=%s\n", value.RawMessage(), expected.RawMessage())

	return value, expected, op
}
