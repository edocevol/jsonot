package jsonot

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

	operations := ot.OperationComponentsFromValue(actions)
	if operations.IsError() {
		t.Errorf("OperationComponentsFromValue failed: %v", operations.Error())
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

func TestRouterGetOnObject(t *testing.T) {
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

	path := Path{Paths: []PathElement{
		{Key: "info"},
		{Key: "extra"},
		//{Key: "some_key_under_extra"},
	}}

	val, err := RouteGetOnValue(&ValueBrian{Value: obj}, path, Object)
	assert.NoError(t, err)
	assert.True(t, val.IsPresent())
	assert.JSONEq(t, `{}`, string(val.MustGet().Value.RawMessage()))
	assert.Equal(t, "extra", val.MustGet().KeyInParent)

	val, err = RouteGetOnValue(&ValueBrian{Value: obj}, path, Array)
	assert.NoError(t, err)
	assert.True(t, val.IsPresent())
	assert.JSONEq(t, `[]`, string(val.MustGet().Value.RawMessage()))
	assert.Equal(t, "extra", val.MustGet().KeyInParent)

	val, err = RouteGetOnValue(&ValueBrian{Value: obj}, path, String)
	assert.NoError(t, err)
	assert.True(t, val.IsPresent())
	assert.JSONEq(t, `""`, string(val.MustGet().Value.RawMessage()))
	assert.Equal(t, "extra", val.MustGet().KeyInParent)
}
