package jsonot

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTransformForListForSamePath tests the transformation of operations for a list
func TestTransformForListForSamePath(t *testing.T) {
	t.Run("insert with no previous operations", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1"}]
[]
[{"p": [1],"li": "v1"}]
[]`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})
	t.Run("insert on same path", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1"}]
[{"p": [1],"li": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"li": "v2"}]`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})
	t.Run("delete on same path", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1", "ld":"v3"}]
[{"p": [1],"li": "v2", "ld":"v4"}]
[{"p": [1],"li": "v1", "ld":"v2"}]
[]`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})

	t.Run("merge delete on same path", func(t *testing.T) {
		line := `
[{"p": [1],"ld": "v1"}]
[{"p": [1],"ld": "v2"}]
[]
[]`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})
}

// TestTransformForListForSamePath tests the transformation of operations for a list
func TestTransformForListWithConflicting(t *testing.T) {
	t.Run("insert conflict with replace", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1"}]
[{"p": [1],"li": "v2", "ld":"v3"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"li": "v2", "ld":"v3"}]
`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})

	t.Run("insert conflict with delete", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1"}]
[{"p": [1],"ld": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"ld": "v2"}]
`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})

	t.Run("replace conflict with delete", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v2", "ld":"v3"}]
[{"p": [1],"ld": "v1"}]
[{"p": [1],"li": "v2"}]
[]
`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})

	t.Run("insert conflict with insert", func(t *testing.T) {
		line := `
[{"p": [1],"li": "v1"}]
[{"p": [1, 2],"li": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2, 2],"li": "v2"}]
`
		ot := NewJSONOperationTransformer()
		left, right, expLeft, expRight := RunTransformTestCase(t, ot, line)
		assert.JSONEq(t, left, expLeft,
			"ApplyListMove case 1 failed, expected %s, got %s", left, expLeft)
		assert.JSONEq(t, right, expRight,
			"ApplyListMove case 1 failed, expected %s, got %s", right, expRight)
	})
}

// TestTransformForDeleteConflict tests the transformation of operations for a delete conflict
func TestTransformForDeleteConflict(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "delete value is already deleted, should merge",
			line: `
[{"p": [1],"ld": {"k1": "v2"}}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"ld": {"k1": "v2"}}]
[]`,
		},
		{
			name: "one deleted values is already deleted, should reduce",
			line: `
[{"p": [1],"ld": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"ld": "v3"}]
[{"p": [1],"ld": ["v1", "v2"]}]
[]`,
		},
		{
			name: "remove total values, should remove all",
			line: `
[{"p": [1],"li": "v1", "ld": {"k2": "v2"}}]
[{"p": [1, 2],"li": "v3", "ld":"v4"}]
[{"p": [1],"li": "v1", "ld": {"k2": "v2"}}]
[]`,
		},
		{
			name: "remove all values should remove previous replaced values",
			line: `
[{"p": [1],"li": "v1", "ld": ["v1","v2","v3"]}]
[{"p": [1, 2],"li": "v4", "ld":"v5"}]
[{"p": [1],"li": "v1", "ld": ["v1","v2", "v4"]}]
[]`,
		},
		{
			name: "remove all values should remove previous replaced values with same path",
			line: `
[{"p": [1],"ld": ["v1", "v2"]}]
[{"p": [1, 2],"li": "v3"}]
[{"p": [1],"ld": ["v1", "v2", "v3"]}]
[]`,
		},
		{
			name: "insert with same path should not conflict",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2, 2],"ld": "v2"}]
`,
		},
		{
			name: "insert with same path should not conflict with delete",
			line: `
[{"p": [1],"li": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"li": ["v1", "v2", "v3"]}]
[{"p": [2, 2],"ld": "v2"}]
`,
		},
		{
			name: "delete index should update to the correct path",
			line: `
[{"p": [1, 2],"ld": "v1"}]
[{"p": [1],"li": {"k2": "v2"}}]
[{"p": [2, 2],"ld": "v1"}]
[{"p": [1],"li": {"k2": "v2"}}]
`,
		},
		{
			name: "list repace index should update to the correct path",
			line: `
[{"p": [1], "li": "v1", "ld": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"li": "v4"}]
[{"p": [1], "li": "v1", "ld": ["v1", "v2", "v4", "v3"]}]
[]
`,
		},
		{
			name: "replace should update to the correct path",
			line: `
[{"p": [1, 2], "li": "v1", "ld": "v2"}]
[{"p": [1],"li": {"k3":"v4"}}]
[{"p": [2, 2], "li": "v1", "ld": "v2"}]
[{"p": [1],"li": {"k3":"v4"}}]
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ot := NewJSONOperationTransformer()
			left, right, expLeft, expRight := RunTransformTestCase(t, ot, test.line)
			assert.JSONEq(t, expLeft, left,
				"ApplyListMove case 1 failed, expected %s, got %s", expLeft, left)
			assert.JSONEq(t, expRight, right,
				"ApplyListMove case 1 failed, expected %s, got %s", expRight, right)
		})
	}
}

// TestTransformForMoveCase1 tests the transformation of operations for a move case
func TestTransformForMoveCase1(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "right side: should move item after insert new one",
			line: `
[{"p": ["k", 0], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0], "li": "v4"}]
[{"p": ["k", 4],"lm": 2}]
`,
		},
		{
			name: "should update insert index after move",
			line: `
[{"p": ["k", 2], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "li": "v4"}]
[{"p": ["k", 4],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path",
			line: `
[{"p": ["k", 3], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 4],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 0, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 1, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 4, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should update insert index after move with same path and same index",
			line: `
[{"p": ["k", 1, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ot := NewJSONOperationTransformer()
			left, right, expLeft, expRight := RunTransformTestCase(t, ot, test.line)
			assert.JSONEq(t, expLeft, left,
				"ApplyListMove case 1 failed, expected %s, got %s", expLeft, left)
			assert.JSONEq(t, expRight, right,
				"ApplyListMove case 1 failed, expected %s, got %s", expRight, right)
		})
	}
}

// TestTransformForDeleteCase1 tests the transformation of operations for a delete case
func TestTransformForDeleteCase1(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should update delete index after move",
			line: `
		[{"p": ["k", 0], "ld": "v4"}]
		[{"p": ["k", 3],"lm": 1}]
		[{"p": ["k", 0], "ld": "v4"}]
		[{"p": ["k", 2],"lm": 0}]
		`,
		},
		{
			name: "should update delete index after move with same path",
			line: `
[{"p": ["k", 2], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 1}]
`,
		},
		{
			line: `
[{"p": ["k", 2], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 1}]
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ot := NewJSONOperationTransformer()
			left, right, expLeft, expRight := RunTransformTestCase(t, ot, test.line)
			assert.JSONEq(t, expLeft, left,
				"ApplyListMove case 1 failed, expected %s, got %s", expLeft, left)
			assert.JSONEq(t, expRight, right,
				"ApplyListMove case 1 failed, expected %s, got %s", expRight, right)
		})
	}
}

// RunApplyTestCase 执行应用测试用例
func RunTransformTestCase(
	t *testing.T, ot *JSONOperationTransformer, lines string,
) (left, right, expLeft, expRight string) {
	operations := ParseTransformCases(t, ot, lines)
	leftOp := operations[0]
	rightOp := operations[1]

	var err error
	leftOp, rightOp, err = ot.Transform(context.Background(), leftOp, rightOp)
	if err != nil {
		t.Fatalf("failed to transform operations: %v", err)
	}

	expectedBaseOp := operations[2]
	expectedOtherOp := operations[3]

	left = string(leftOp.ToNode().RawMessage())
	right = string(rightOp.ToNode().RawMessage())
	expLeft = string(expectedBaseOp.ToNode().RawMessage())
	expRight = string(expectedOtherOp.ToNode().RawMessage())

	return left, right, expLeft, expRight
}

// ReadTransformCaseFromFile reads transformation cases from a file
func ReadTransformCaseFromFile(t *testing.T, fileName string) [][]*Operation {
	fd, err := os.Open("testdata/" + fileName)
	if err != nil {
		t.Fatalf("failed to open file %s: %v", fileName, err)
	}
	defer fd.Close()

	// 按照 4 行一组来读取，过滤掉空行，带有注释的行
	var cases [][]*Operation
	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue // 跳过空行和注释行
		}

		if len(cases) == 0 || len(cases[len(cases)-1]) == 4 {
			cases = append(cases, make([]*Operation, 0, 4))
		}

		op := ParseTransformCases(t, NewJSONOperationTransformer(), line)
		cases[len(cases)-1] = append(cases[len(cases)-1], op...)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("failed to read file %s: %v", fileName, err)
	}

	return cases
}

// ParseTransformCases parses the transformation cases from a string
// 每个测试用例一共是 4 行，对前 2 行做变换，最后两行是预期结果
// [{"p": [1],"li": "v1"}]
// [{"p": [1],"li": "v2"}]
// [{"p": [1],"li": "v1"}]
// [{"p": [2],"li": "v2"}]
func ParseTransformCases(t *testing.T, ot *JSONOperationTransformer, lines string) []*Operation {
	// 去除空行
	lines = strings.TrimSpace(lines)
	cases := strings.Split(lines, "\n")
	if len(cases) != 4 {
		t.Fatalf("failed to parse transform case, expected 4 lines, got %d", len(cases))
	}

	var operations []*Operation
	for k := range cases {
		opNode, err := UnmarshalValue([]byte(cases[k]))
		if err != nil {
			t.Fatalf("failed to parse operation node: %v", err)
		}
		log.Debugf("got operation node: %s\n", opNode.RawMessage())
		op := NewOperation(ot.OperationComponentsFromNode(opNode).MustGet())
		operations = append(operations, op)
	}

	return operations
}
