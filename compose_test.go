package jsonot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestComposeCase1 测试 Compose
func TestComposeCase1(t *testing.T) {
	lines := `
[{"p":["p1"], "na":600}]
[{"p":["p1"], "na":100}]
[{"p":["p1"], "na":700}]
`

	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase2 测试 Compose
func TestComposeCase2(t *testing.T) {
	lines := `
[{"p":["p1"], "na":100.0}]
[{"p":["p1"], "na":-100}]
[{"p":["p1"], "na":0.0}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase3 测试 Compose
func TestComposeCase3(t *testing.T) {
	lines := `
[{"p":["p1"], "na":100.0}]
[{"p":["p1", "p2"], "na":-100.0}]
[{"p":["p1"], "na":100.0},{"p":["p1", "p2"], "na":-100.0}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase4 测试 Compose
func TestComposeCase4(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}, {"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase5 测试 Compose
func TestComposeCase5(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"worldhello"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase6 测试 Compose
func TestComposeCase6(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":4, "i":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"heworldllo"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase7 测试 Compose
func TestComposeCase7(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":7, "i":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"helloworld"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase8 测试 Compose
func TestComposeCase8(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":1, "i":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}, {"p":["p1"], "t":"text", "o":{"p":1, "i":"world"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase9 测试 Compose
func TestComposeCase9(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":8, "i":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}, {"p":["p1"], "t":"text", "o":{"p":8, "i":"world"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase10 测试 Compose
func TestComposeCase10(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1", "p2"], "t":"text", "o":{"p":2, "i":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "i":"hello"}},{"p":["p1", "p2"], "t":"text", "o":{"p":2, "i":"hello"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase11 测试 Compose
func TestComposeCase11(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"helloworld"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase12 测试 Compose
func TestComposeCase12(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":4, "d":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"wohellorld"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase13 测试 Compose
func TestComposeCase13(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":7, "d":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"worldhello"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase14 测试 Compose
func TestComposeCase14(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":1, "d":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":1, "d":"hello"}}, {"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// TestComposeCase15 测试 Compose
func TestComposeCase15(t *testing.T) {
	lines := `
[{"p":["p1"], "t":"text", "o":{"p":8, "d":"hello"}}]
[{"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
[{"p":["p1"], "t":"text", "o":{"p":8, "d":"hello"}}, {"p":["p1"], "t":"text", "o":{"p":2, "d":"world"}}]
`
	actual, expected := RunComposeTest(t, lines)
	assert.JSONEq(t, expected, actual)
}

// ParseLines 解析多行字符串为节点列表
func ParseLines(t *testing.T, lines string) []Value {
	var actions []Value
	cases := strings.Split(lines, "\n")
	for _, line := range cases {
		if line == "" {
			continue
		}
		node, err := UnmarshalValue([]byte(line))
		if err != nil {
			t.Fatalf("ParseLines error: %v", err)
		}
		actions = append(actions, node)
	}

	return actions
}

// RunComposeTest 执行 Compose 测试
func RunComposeTest(t *testing.T, lines string) (actual, expected string) {
	actions := ParseLines(t, lines)
	if len(actions) < 2 {
		t.Skipf("Not enough actions: %d", len(actions))
	}

	ot := NewJSONOperationTransformer()
	baseOperationComponents := ot.OperationComponentsFromNode(actions[0])
	baseOperation := NewOperation(baseOperationComponents.MustGet())
	for i := 1; i < len(actions)-1; i++ {
		nextOperation := ot.OperationComponentsFromNode(actions[i])
		if nextOperation.IsError() {
			t.Errorf("OperationComponentsFromNode failed: %v", nextOperation.Error())
			return
		}
		nextOp := NewOperation(nextOperation.MustGet())
		baseOperation.Compose(nextOp)
	}

	expectedOperation := NewOperation(ot.OperationComponentsFromNode(actions[len(actions)-1]).MustGet())
	actual = string(baseOperation.ToNode().RawMessage())
	expected = string(expectedOperation.ToNode().RawMessage())
	return
}
