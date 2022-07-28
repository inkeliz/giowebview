package giowebview

import (
	"sync"

	"gioui.org/io/pointer"
	"gioui.org/op"
	"gioui.org/op/clip"
)

type pool[T any] struct {
	pool sync.Pool
}

func newPool[T any](fn func() any) pool[T] {
	return pool[T]{pool: sync.Pool{New: fn}}
}

func (x *pool[T]) add(op *op.Ops, data T) {
	cmd := x.pool.New()
	*cmd.(*T) = data

	defer clip.Rect{}.Push(op).Pop()
	pointer.InputOp{Tag: cmd}.Add(op)
}

func (x *pool[T]) free(data *T) (v T) {
	*data = v
	x.pool.Put(data)
	return v
}
