package gorpc

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
)

type Foo int

type Args struct {
	a int
	b int
}

func (f *Foo) Sum(arg Args, reply *int) error {
	*reply = arg.a + arg.b
	return nil
}

func (f *Foo) sum(arg Args, reply *int) error {
	*reply = arg.a + arg.b
	return nil
}

func _assert(condition bool, errstr string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assert failed: "+errstr, v...))
	}
}

func TestNewService(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	logrus.Info(s)
	_assert(len(s.method) == 1, "wrong service number, expected 1, got %d", len(s.method))
	methodType := s.method["Sum"]
	_assert(methodType != nil, "wrong method")
}

func TestMethodType_Call(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	mt := s.method["Sum"]
	argv := mt.newArgv()
	replyv := mt.newReplyv()
	logrus.Info(mt)
	argv.Set(reflect.ValueOf(Args{1, 2}))
	err := s.Call(mt, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 3 && mt.numCalls == 1, "fail to call Foo.Sum")
}
