package jsonot

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONOperationTransformer_DiffNestedDocument(t *testing.T) {
	ot := NewJSONOperationTransformer()

	current, err := UnmarshalValue([]byte(`{"name":"v2","count":5,"items":[1,2],"extra":true}`))
	assert.NoError(t, err)
	target, err := UnmarshalValue([]byte(`{"name":"v1","count":3,"items":[1,4],"new":"x"}`))
	assert.NoError(t, err)

	operation := ot.Diff(context.Background(), current, target)
	assert.True(t, operation.IsOk())

	result := ot.Apply(context.Background(), current, operation.MustGet())
	assert.True(t, result.IsOk())
	assert.JSONEq(t, `{"name":"v1","count":3,"items":[1,4],"new":"x"}`, string(result.MustGet().RawMessage()))
}

func TestJSONOperationTransformer_DiffArrayFallbackReplace(t *testing.T) {
	ot := NewJSONOperationTransformer()

	current, err := UnmarshalValue([]byte(`{"items":[1,2,3]}`))
	assert.NoError(t, err)
	target, err := UnmarshalValue([]byte(`{"items":[1,3]}`))
	assert.NoError(t, err)

	operation := ot.Diff(context.Background(), current, target)
	assert.True(t, operation.IsOk())
	assert.JSONEq(t, `[{"p":["items"],"oi":[1,3],"od":[1,2,3]}]`, string(operation.MustGet().ToValue().RawMessage()))

	result := ot.Apply(context.Background(), current, operation.MustGet())
	assert.True(t, result.IsOk())
	assert.JSONEq(t, `{"items":[1,3]}`, string(result.MustGet().RawMessage()))
}

func TestJSONOperationTransformer_DiffRootReplace(t *testing.T) {
	ot := NewJSONOperationTransformer()

	current, err := UnmarshalValue([]byte(`[1,2,3]`))
	assert.NoError(t, err)
	target, err := UnmarshalValue([]byte(`{"version":1}`))
	assert.NoError(t, err)

	operation := ot.Diff(context.Background(), current, target)
	assert.True(t, operation.IsOk())
	assert.JSONEq(t, `[{"p":[],"oi":{"version":1},"od":[1,2,3]}]`, string(operation.MustGet().ToValue().RawMessage()))

	result := ot.Apply(context.Background(), current, operation.MustGet())
	assert.True(t, result.IsOk())
	assert.JSONEq(t, `{"version":1}`, string(result.MustGet().RawMessage()))
}

func TestJSONOperationTransformer_DiffRootNumberAdd(t *testing.T) {
	ot := NewJSONOperationTransformer()

	current, err := UnmarshalValue([]byte(`5`))
	assert.NoError(t, err)
	target, err := UnmarshalValue([]byte(`2`))
	assert.NoError(t, err)

	operation := ot.Diff(context.Background(), current, target)
	assert.True(t, operation.IsOk())
	assert.JSONEq(t, `[{"p":[],"na":-3}]`, string(operation.MustGet().ToValue().RawMessage()))

	result := ot.Apply(context.Background(), current, operation.MustGet())
	assert.True(t, result.IsOk())
	assert.JSONEq(t, `2`, string(result.MustGet().RawMessage()))
}
