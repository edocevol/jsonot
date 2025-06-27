package jsonot

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTransformForDeleteConflict1 tests the transformation of operations for a delete conflict
func TestTransformForDeleteConflict1(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "list_insert_with_no_previous_operations",
			line: `
[{"p": [1],"li": "v1"}]
[]
[{"p": [1],"li": "v1"}]
[]
`,
		},
		{
			name: "re_insert_value_with_previous_operations",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1],"li": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"li": "v2"}]
`,
		},
		{
			name: "re_repace_value_with_previous_operations",
			line: `
[{"p": [1],"li": "v1", "ld":"v3"}]
[{"p": [1],"li": "v2", "ld":"v4"}]
[{"p": [1],"li": "v1", "ld":"v2"}]
[]
`,
		},
		{
			name: "re_delete_value_with_previous_operations",
			line: `
[{"p": [1],"ld": "v1"}]
[{"p": [1],"ld": "v2"}]
[]
[]
`,
		},
		{
			name: "re_insert_value_with_previous_replace_operations",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1],"li": "v2", "ld":"v3"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"li": "v2", "ld":"v3"}]
`,
		},
		{
			name: "insert_same_path_with_previous_delete_operations",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1],"ld": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2],"ld": "v2"}]
`,
		},
		{
			name: "repace_same_path_with_previous_delete_operations",
			line: `
[{"p": [1],"li": "v2", "ld":"v3"}]
[{"p": [1],"ld": "v1"}]
[{"p": [1],"li": "v2"}]
[]
`,
		},
		{
			name: "reinsert_value_for_full_path_updated_by_previous_sub_update",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1, 2],"li": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2, 2],"li": "v2"}]
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

// TestTransformForDeleteConflict2 tests the transformation of operations for a delete conflict
func TestTransformForDeleteConflict2(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "reinsert_value_for_full_path_updated_by_previous_sub_update",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1, 2],"li": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2, 2],"li": "v2"}]
`,
		},
		{
			name: "delete_value_is_already_deleted_should_merge",
			line: `
[{"p": [1],"ld": {"k1": "v2"}}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"ld": {"k1": "v2"}}]
[]`,
		},
		{
			name: "one_deleted_values_is_already_deleted_should_reduce",
			line: `
[{"p": [1],"ld": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"ld": "v3"}]
[{"p": [1],"ld": ["v1", "v2"]}]
[]`,
		},
		{
			name: "remove_total_values_should_remove_all",
			line: `
[{"p": [1],"li": "v1", "ld": {"k2": "v2"}}]
[{"p": [1, 2],"li": "v3", "ld":"v4"}]
[{"p": [1],"li": "v1", "ld": {"k2": "v2"}}]
[]`,
		},
		{
			name: "remove_all_values_should_remove_previous_replaced_values",
			line: `
[{"p": [1],"li": "v1", "ld": ["v1","v2","v3"]}]
[{"p": [1, 2],"li": "v4", "ld":"v5"}]
[{"p": [1],"li": "v1", "ld": ["v1","v2", "v4"]}]
[]`,
		},
		{
			name: "remove_all_values_should_remove_previous_replaced_values_with_same_path",
			line: `
[{"p": [1],"ld": ["v1", "v2"]}]
[{"p": [1, 2],"li": "v3"}]
[{"p": [1],"ld": ["v1", "v2", "v3"]}]
[]`,
		},
		{
			name: "insert_with_same_path_should_not_conflict",
			line: `
[{"p": [1],"li": "v1"}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"li": "v1"}]
[{"p": [2, 2],"ld": "v2"}]
`,
		},
		{
			name: "insert_with_same_path_should_not_conflict_with_delete",
			line: `
[{"p": [1],"li": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"ld": "v2"}]
[{"p": [1],"li": ["v1", "v2", "v3"]}]
[{"p": [2, 2],"ld": "v2"}]
`,
		},
		{
			name: "delete_index_should_update_to_the_correct_path",
			line: `
[{"p": [1, 2],"ld": "v1"}]
[{"p": [1],"li": {"k2": "v2"}}]
[{"p": [2, 2],"ld": "v1"}]
[{"p": [1],"li": {"k2": "v2"}}]
`,
		},
		{
			name: "list_repace_index_should_update_to_the_correct_path",
			line: `
[{"p": [1], "li": "v1", "ld": ["v1", "v2", "v3"]}]
[{"p": [1, 2],"li": "v4"}]
[{"p": [1], "li": "v1", "ld": ["v1", "v2", "v4", "v3"]}]
[]
`,
		},
		{
			name: "replace_should_update_to_the_correct_path",
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
			name: "right_side_should_move_item_after_insert_new_one",
			line: `
[{"p": ["k", 0], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0], "li": "v4"}]
[{"p": ["k", 4],"lm": 2}]
`,
		},
		{
			name: "should_update_insert_index_after_move",
			line: `
[{"p": ["k", 2], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "li": "v4"}]
[{"p": ["k", 4],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path",
			line: `
[{"p": ["k", 3], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 4],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index",
			line: `
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_2",
			line: `
[{"p": ["k", 0, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_3",
			line: `
[{"p": ["k", 1, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_4",
			line: `
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_5",
			line: `
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_6",
			line: `
[{"p": ["k", 3, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 2, 1], "li": "v4"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_7",
			line: `
[{"p": ["k", 4, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4, 1], "li": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_insert_index_after_move_with_same_path_and_same_index_8",
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

// TestTransformForAfterMoveCase1 tests the transformation of operations for a delete case
func TestTransformForAfterMoveCase1(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_update_delete_index_after_move",
			line: `
		[{"p": ["k", 0], "ld": "v4"}]
		[{"p": ["k", 3],"lm": 1}]
		[{"p": ["k", 0], "ld": "v4"}]
		[{"p": ["k", 2],"lm": 0}]
		`,
		},
		{
			name: "should_update_delete_index_after_move_with_same_path",
			line: `
[{"p": ["k", 2], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 1}]
`,
		},
		{
			name: "should_update_delete_index_after_move_with_same_path_and_same_index",
			line: `
[{"p": ["k", 2], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 1}]
`,
		},
		{
			name: "should_update_index_after_element_has_been_moved", // 其实挺危险的
			line: `
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1], "ld": "v4"}]
[]
`,
		},
		{
			name: "the_front_element_move_to_front_should_not_change_index",
			line: `
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 0}]
[{"p": ["k", 3], "ld": "v4"}]
[{"p": ["k", 2],"lm": 0}]
`,
		},
		{
			name: "the_back_element_move_to_back_should_not_change_index",
			line: `
[{"p": ["k", 0, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "the_back_element_move_to_current_should_update_index",
			line: `
[{"p": ["k", 1, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 2, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "the_back_element_move_to_front_should_update_index",
			line: `
[{"p": ["k", 2, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
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

// TestTransformForAfterMoveCase2 tests the transformation of operations for a delete case
func TestTransformForAfterMoveCase2(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_update_index_after_element_has_been_moved_to_front",
			line: `
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "update_sub_path_should_not_change_index_after_parent_peer_move",
			line: `
[{"p": ["k", 4, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4, 1], "ld": "v4"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_front_element_move_to_current",
			line: `
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 2, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
		{
			name: "should_update_index_when_front_element_move_to_front_current",
			line: `
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 2}]
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 2}]
`,
		},
		{
			name: "should_update_sub_index_after_sub_index_has_been_moved",
			line: `
[{"p": ["k", 1, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 3, 1], "ld": "v4"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
		{
			name: "should_update_index_when_current_element_has_been_moved_to_back",
			line: `
[{"p": ["k", 1, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
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

// TesTransformForListMoveAfterMove tests the transformation of operations for a list move after move
func TestTransformForListMoveAfterMove(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "move_item_in_place_after_move",
			line: `
[{"p": ["k", 0], "lm": 0}]
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 0], "lm": 0}]
[{"p": ["k", 0], "lm": 1}]
`,
		},
		{
			name: "reduce_same_move_operation_after_move_case1",
			line: `
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 0], "lm": 1}]
[]
[]
`,
		},
		{
			name: "reduce_same_move_operation_after_move_case2",
			line: `
[{"p": ["k", 1], "lm": 0}]
[{"p": ["k", 1], "lm": 0}]
[]
[]
`,
		},
		{
			name: "should_update_index_after_current_element_has_been_moved_case1",
			line: `
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 0], "lm": 2}]
[{"p": ["k", 2], "lm": 1}]
[]
`,
		},
		{
			name: "should_update_index_after_current_element_has_been_moved_case2",
			line: `
[{"p": ["k", 4], "lm": 2}]
[{"p": ["k", 4], "lm": 3}]
[{"p": ["k", 3], "lm": 2}]
[]
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

// TestTransformForListMoveAfterMoveWithNopOverlap tests the transformation of operations
// for a list move after move with no overlap

func TestTransformForListMoveAfterMoveWithNopOverlap(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_update_index_after_current_element_has_been_moved_case1",
			line: `
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 0], "lm": 2}]
[{"p": ["k", 2], "lm": 1}]
[]
`,
		},
		{
			name: "should_not_update_index_course_previous_has_different_operation_range_case1",
			line: `
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 3], "lm": 4}]
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 3], "lm": 4}]
`,
		},
		{
			name: "should_not_update_index_course_previous_has_different_operation_range_case2",
			line: `
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 4], "lm": 3}]
[{"p": ["k", 0], "lm": 1}]
[{"p": ["k", 4], "lm": 3}]
`,
		},
		{
			name: "should_not_update_index_course_previous_has_different_operation_range_case3",
			line: `
[{"p": ["k", 1], "lm": 0}]
[{"p": ["k", 3], "lm": 4}]
[{"p": ["k", 1], "lm": 0}]
[{"p": ["k", 3], "lm": 4}]
`,
		},
		{
			name: "should_not_update_index_course_previous_has_different_operation_range_case4",
			line: `
[{"p": ["k", 1], "lm": 0}]
[{"p": ["k", 4], "lm": 3}]
[{"p": ["k", 1], "lm": 0}]
[{"p": ["k", 4], "lm": 3}]
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

// TestTransformForListMoveAfterMoveWithOverlapAndInclusive tests the transformation of operations
// for a list move after move with overlap and inclusive
func TestTransformForListMoveAfterMoveWithOverlapAndInclusive(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_update_right_index_current_element_move_to_back_case1",
			line: `
[{"p": ["k", 0], "lm": 4}]
[{"p": ["k", 2], "lm": 3}]
[{"p": ["k", 0], "lm": 4}]
[{"p": ["k", 1], "lm": 2}]
`,
		},
		{
			name: "should_update_right_index_current_element_move_to_back_case2",
			line: `
[{"p": ["k", 0], "lm": 4}]
[{"p": ["k", 3], "lm": 2}]
[{"p": ["k", 0], "lm": 4}]
[{"p": ["k", 2], "lm": 1}]
`,
		},
		{
			name: "should_update_right_index_current_element_move_to_back_case3",
			line: `
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 2], "lm": 3}]
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 3], "lm": 4}]
`,
		},
		{
			name: "should_update_right_index_current_element_move_to_back_case4",
			line: `
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 3], "lm": 2}]
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 4], "lm": 3}]
`,
		},
		{
			name: "should_update_right_index_current_element_move_to_back_case5",
			line: `
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 3], "lm": 0}]
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 4], "lm": 1}]
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

// TestTransformForListMoveAfterMoveWithOverlapAndIntersect tests the transformation of operations
// for a list move after move with overlap and intersect
func TestTransformForListMoveAfterMoveWithOverlapAndIntersect(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_update_both_index_when_exclusive_intersect",
			line: `
[{"p": ["k", 0], "lm": 3}]
[{"p": ["k", 2], "lm": 4}]
[{"p": ["k", 0], "lm": 2}]
[{"p": ["k", 1], "lm": 4}]
`,
		},
		{
			name: "should_increase_left_index_when_back_element_move_to_front",
			line: `
[{"p": ["k", 0], "lm": 3}]
[{"p": ["k", 4], "lm": 2}]
[{"p": ["k", 0], "lm": 4}]
[{"p": ["k", 4], "lm": 1}]
`,
		},
		{
			name: "should_decrease_left_index_when_current_element_move_to_back",
			line: `
[{"p": ["k", 3], "lm": 0}]
[{"p": ["k", 2], "lm": 4}]
[{"p": ["k", 2], "lm": 0}]
[{"p": ["k", 3], "lm": 4}]
`,
		},
		{
			name: "should_update_both_index_when_exclusive_intersect_case2",
			line: `
[{"p": ["k", 3], "lm": 0}]
[{"p": ["k", 4], "lm": 2}]
[{"p": ["k", 4], "lm": 0}]
[{"p": ["k", 4], "lm": 3}]
`,
		},
		{
			name: "should_update_both_index_with_inclusive_intersect_case1",
			line: `
[{"p": ["k", 0], "lm": 3}]
[{"p": ["k", 3], "lm": 0}]
[{"p": ["k", 1], "lm": 3}]
[{"p": ["k", 2], "lm": 0}]
`,
		},
		{
			name: "should_update_both_index_with_inclusive_intersect_case2",
			line: `
[{"p": ["k", 0], "lm": 3}]
[{"p": ["k", 3], "lm": 4}]
[{"p": ["k", 0], "lm": 2}]
[{"p": ["k", 2], "lm": 4}]
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

// TestTransformForObjectReplaceAfterMove tests the transformation of operations for an object replace after move
func TestTransformForObjectReplaceAfterMove(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "should_not_update_index_when_back_element_move_to_back_of_current",
			line: `
[{"p": ["k", 0], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_back_element_move_to_front_of_current",
			line: `
[{"p": ["k", 2], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_current_element_move_to_front_of_current",
			line: `
[{"p": ["k", 3], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_not_update_index_when_front_element_move_to_front_of_current",
			line: `
[{"p": ["k", 3], "ld": "v4", "li":"v5"}]
[{"p": ["k", 2],"lm": 0}]
[{"p": ["k", 3], "ld": "v4", "li":"v5"}]
[{"p": ["k", 2],"lm": 0}]
`,
		},
		{
			name: "should_not_update_index_when_back_element_move_to_back_of_current",
			line: `
[{"p": ["k", 0, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 0, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_back_element_move_to_current_index",
			line: `
[{"p": ["k", 1, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 2, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_back_element_move_to_front_index",
			line: `
[{"p": ["k", 2, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_update_index_when_current_element_move_to_front_index",
			line: `
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 1, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_not_update_index_when_front_element_move_to_front_index",
			line: `
[{"p": ["k", 4, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
[{"p": ["k", 4, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 3],"lm": 1}]
`,
		},
		{
			name: "should_decrease_index_when_front_element_move_to_current_index",
			line: `
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 1],"lm": 3}]
[{"p": ["k", 2, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 1],"lm": 3}]
`,
		},
		{
			name: "should_not_update_index_when_front_element_move_to_front_index",
			line: `
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 1],"lm": 2}]
[{"p": ["k", 3, 1], "ld": "v4", "li":"v5"}]
[{"p": ["k", 1],"lm": 2}]
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

// TestTransformForObjectCaseSet1 tests the transformation of operations for an object case
func TestTransformForObjectCaseSet1(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "object_insert_with_no_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[]
[{"p": ["p1"],"oi": "v1"}]
[]
`,
		},
		{
			name: "object_insert_different_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p2"],"oi": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p2"],"oi": "v2"}]
`,
		},
		{
			name: "object_insert_has_different_sub_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1", "p3"],"oi": "v2"}]
[{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1", "p3"],"oi": "v2"}]
`,
		},
		{
			name: "object_replace_different_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1", "od":"v3"}]
[{"p": ["p2"],"oi": "v2", "od":"v4"}]
[{"p": ["p1"],"oi": "v1", "od":"v3"}]
[{"p": ["p2"],"oi": "v2", "od":"v4"}]
`,
		},
		{
			name: "object_insert_has_sub_difference_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"oi": "v1", "od":"v3"}]
[{"p": ["p1", "p3"],"oi": "v2", "od":"v4"}]
[{"p": ["p1", "p2"],"oi": "v1", "od":"v3"}]
[{"p": ["p1", "p3"],"oi": "v2", "od":"v4"}]
`,
		},
		{
			name: "object_delete_different_with_previous_operations",
			line: `
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p2"],"od": "v2"}]
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p2"],"od": "v2"}]
`,
		},
		{
			name: "object_delete_has_different_sub_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1", "p3"],"od": "v2"}]
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1", "p3"],"od": "v2"}]
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

// TestTransformForObjectCaseSet2 tests the transformation of operations for an object case
func TestTransformForObjectCaseSet2(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "object_insert_conflict_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2"}]
[{"p": ["p1"],"oi": "v1", "od":"v2"}]
[]
`,
		},
		{
			name: "object_insert_different_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p2"],"oi": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p2"],"oi": "v2"}]
`,
		},
		{
			name: "object_insert_has_different_sub_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1", "p3"],"oi": "v2"}]
[{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1", "p3"],"oi": "v2"}]
`,
		},
		{
			name: "object_replace_different_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1", "od":"v3"}]
[{"p": ["p2"],"oi": "v2", "od":"v4"}]
[{"p": ["p1"],"oi": "v1", "od":"v3"}]
[{"p": ["p2"],"oi": "v2", "od":"v4"}]
`,
		},
		{
			name: "object_insert_has_sub_difference_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"oi": "v1", "od":"v3"}]
[{"p": ["p1", "p3"],"oi": "v2", "od":"v4"}]
[{"p": ["p1", "p2"],"oi": "v1", "od":"v3"}]
[{"p": ["p1", "p3"],"oi": "v2", "od":"v4"}]
`,
		},
		{
			name: "object_delete_different_with_previous_operations",
			line: `
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p2"],"od": "v2"}]
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p2"],"od": "v2"}]
`,
		},
		{
			name: "object_delete_has_different_sub_path_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1", "p3"],"od": "v2"}]
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1", "p3"],"od": "v2"}]
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

// TestTransformForObjectCaseSet3 tests the transformation of operations for an object case
func TestTransformForObjectCaseSet3(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "object_insert_conflict_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2"}]
[{"p": ["p1"],"oi": "v1", "od":"v2"}]
[]
`,
		},
		{
			name: "object_replace_conflict_with_previous_operations",
			line: `
[{"p": ["p1"],"oi": "v1", "od":"v3"}]
[{"p": ["p1"],"oi": "v2", "od":"v4"}]
[{"p": ["p1"],"oi": "v1", "od":"v2"}]
[]
`,
		},
		{
			name: "object_delete_conflict_with_previous_operations",
			line: `
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p1"],"od": "v2"}]
[]
[]
`,
		},
		{
			name: "object_insert_conflict_with_previous_operations_with_same_path",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2", "od":"v3"}]
[{"p": ["p1"],"oi": "v1", "od":"v2"}]
[]
`,
		},
		{
			name: "object_insert_replace_conflict_with_previous_insert_operations_with_same_path",
			line: `
[{"p": ["p1"],"oi": "v2", "od":"v3"}]
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2", "od":"v1"}]
[]
`,
		},
		{
			name: "object_insert_conflict_with_previous_delete_operations_with_same_path",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"od": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[]
`,
		},
		{
			name: "object_delete_replace_conflict_with_previous_insert_operations_with_same_path",
			line: `
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1"],"od": "v1"}]
[]
`,
		},
		{
			name: "object_replace_conflict_with_previous_replace_operations_with_same_path",
			line: `
[{"p": ["p1"],"oi": "v2", "od":"v3"}]
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p1"],"oi": "v2"}]
[]
`,
		},
		{
			name: "object_delete_conflict_with_previous_replace_operations_with_same_path",
			line: `
[{"p": ["p1"],"od": "v1"}]
[{"p": ["p1"],"oi": "v2", "od":"v3"}]
[]
[]
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ot := NewJSONOperationTransformer()
			left, right, expLeft, expRight := RunTransformTestCase(t, ot, test.line)
			assert.JSONEq(t, expLeft, left,
				"transform left value mismatch, expected %s, got %s", expLeft, left)
			assert.JSONEq(t, expRight, right,
				"transform right value mismatch, expected %s, got %s", expRight, right)
		})
	}
}

// TestTransformForObjectCaseSet4 测试路径覆盖情况下的 object 操作变换
func TestTransformForObjectCaseSet4(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "object_insert_conflict_with_previous_operations_with_sub_path",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1", "p2"],"oi": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[]
`,
		},
		{
			name: "object_insert_on_sub_path_conflict_with_previous_operations",
			line: `
[{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2"}]
[{"p": ["p1"], "od":"v2"},{"p": ["p1", "p2"],"oi": "v1"}]
[{"p": ["p1"],"oi": "v2"}]
`,
		},
		{
			name: "delete_conflict_with_insert_operations_on_sub_path",
			line: `
[{"p": ["p1"],"od": {"p2":"v2"}}]
[{"p": ["p1", "p2"],"od": "v2"}]
[{"p": ["p1"],"od": {}}]
[]
`,
		},
		{
			name: "replace_conflict_with_replace_operations_on_sub_path",
			line: `
[{"p": ["p1"],"oi": "v1", "od": {"p2": "v2"}}]
[{"p": ["p1", "p2"],"oi": "v3", "od":"v4"}]
[{"p": ["p1"],"oi": "v1", "od": {"p2": "v3"}}]
[]
`,
		},
		{
			name: "delete_key_should_update_when_previous_operation_insert_on_sub_path",
			line: `
[{"p": ["p1"],"od": {"p2": "v1"}}]
[{"p": ["p1", "p2"],"oi": "v2"}]
[{"p": ["p1"],"od": {"p2": "v2"}}]
[]
`,
		},
		{
			name: "insert_on_root_path_should_ignore_previous_delete_on_sub_path",
			line: `
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1", "p2"],"od": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[]
`,
		},
		{
			name: "delete_on_other_sub_path_should_ignore_previous_insert_on_sub_path",
			line: `
[{"p": ["p1", "p2"],"od": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
[{"p": ["p1", "p2"],"od": "v2"}]
[{"p": ["p1"],"oi": "v1"}]
`,
		},
		{
			name: "delete_the_sub_path_which_is_inserted_by_previous_operation",
			line: `
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1"],"oi": {"p2": "v2"}}]
[{"p": ["p1", "p2"],"od": "v1"}]
[{"p": ["p1"],"oi": {"p2": "v2"}}]
`,
		},
		{
			name: "replace_the_sub_path_which_is_updated_by_previous_operation",
			line: `
[{"p": ["p1"], "oi": "v1", "od": {"p2": "v2"}}]
[{"p": ["p1", "p2"],"oi": "v3"}]
[{"p": ["p1"], "oi": "v1", "od": {"p2": "v3"}}]
[]
`,
		},
		{
			name: "replace_on_sub_path_should_delete_full_value_which_is_inserted_by_previous_operation",
			line: `
[{"p": ["p1", "p2"], "oi": "v1", "od": "v2"}]
[{"p": ["p1"],"oi": {"p3":"v4"}}]
[{"p":["p1"], "od": {"p3":"v4"}}, {"p": ["p1", "p2"], "oi": "v1", "od": "v2"}]
[{"p": ["p1"],"oi": {"p3":"v4"}}]
`,
		},
		{
			name: "delete_on_sub_path_which_is_not_unset_by_previous_operation",
			line: `
[{"p": ["p1", "p2"], "od": "v1"}]
[{"p": ["p1"],"oi": "v2", "od": "v3"}]
[]
[{"p": ["p1"],"oi": "v2", "od": "v3"}]
`,
		},
		{
			name: "insert_should_empty_if_the_value_is_deleted_by_previous_operation",
			line: `
[{"p": ["p1"], "li": "v1"}]
[{"p": ["p1"],"od": ["l3","l4"]}]
[]
[{"p": ["p1"],"od": ["l3","l4"]}]
`,
		},
		{
			name: "insert_should_empty_if_the_value_is_replaced_by_previous_operation",
			line: `
[{"p": ["p1"], "li": "v1"}]
[{"p": ["p1"],"od": ["l3","l4"], "oi":["l5","l6"]}]
[]
[{"p": ["p1"],"od": ["l3","l4"], "oi":["l5","l6"]}]
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ot := NewJSONOperationTransformer()
			left, right, expLeft, expRight := RunTransformTestCase(t, ot, test.line)
			assert.JSONEq(t, expLeft, left,
				"transform left value mismatch, expected %s, got %s", expLeft, left)
			assert.JSONEq(t, expRight, right,
				"transform right value mismatch, expected %s, got %s", expRight, right)
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

	left = string(leftOp.ToValue().RawMessage())
	right = string(rightOp.ToValue().RawMessage())
	expLeft = string(expectedBaseOp.ToValue().RawMessage())
	expRight = string(expectedOtherOp.ToValue().RawMessage())

	return left, right, expLeft, expRight
}

// ReadTransformCaseFromFile reads transformation cases from a file
func ReadTransformCaseFromFile(t *testing.T, fileName string) [][]*Operation {
	fd, err := os.Open("testdata/" + fileName)
	if err != nil {
		t.Fatalf("failed to open file %s: %v", fileName, err)
	}
	defer func() {
		_ = fd.Close()
	}()

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
		op := NewOperation(ot.OperationComponentsFromValue(opNode).MustGet())
		operations = append(operations, op)
	}

	return operations
}
