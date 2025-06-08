package jsonot

import (
	"context"
	"testing"
)

func TestJSONOperationTransformer_Apply(t *testing.T) {
	ot := NewJSONOperationTransformer()

	obj, _ := UnmarshalValue([]byte(`{
      "name": "json0",
      "age": 18,
      "is_student": true,
      "hobbies": ["reading", "coding", "music"],
      "info": {
          "address": "China",
          "email": "example@mail.qq.com"
      }
    }`))

	actions, _ := UnmarshalValue([]byte(`
[
	{"p": ["hobbies", "2"], "ld": "music"},
	{"p": ["hobbies", "3"], "li": "movie"},
	{"p": ["info", "email"], "od": "example@mail.qq.com"}
]`))

	operations := ot.OperationComponentsFromNode(actions)
	if operations.IsError() {
		t.Errorf("OperationComponentsFromNode failed: %v", operations.Error())
		return
	}

	operator := NewOperation(operations.MustGet())
	result := ot.Apply(context.Background(), obj, operator)
	if result.IsError() {
		t.Errorf("Apply failed: %v", result.Error())
		return
	}

	t.Logf("Apply result: %s", result.MustGet().RawMessage())
}
