package gorpc

import (
	"go/ast"
	"reflect"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

type MethodType struct {
	method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	numCalls  uint64
}

func (mt *MethodType) NumCalls() uint64 {
	return atomic.LoadUint64(&mt.numCalls)
}

func (mt *MethodType) newArgv() reflect.Value {
	var v reflect.Value
	if mt.ArgType.Kind() == reflect.Ptr {
		v = reflect.New(mt.ArgType.Elem())
	} else {
		v = reflect.New(mt.ArgType).Elem()
	}
	return v
}

func (mt *MethodType) newReplyv() reflect.Value {
	v := reflect.New(mt.ReplyType.Elem())
	if mt.ReplyType.Elem().Kind() == reflect.Map {
		v.Elem().Set(reflect.MakeMap(mt.ReplyType.Elem()))
	} else if mt.ReplyType.Elem().Kind() == reflect.Slice {
		v.Elem().Set(reflect.MakeSlice(mt.ReplyType.Elem(), 0, 0))
	}
	return v
}

type service struct {
	name          string
	serviceType   reflect.Type
	serviceStruct reflect.Value
	method        map[string]*MethodType
}

func NewService(serviceStruct interface{}) *service {
	s := new(service)
	s.serviceStruct = reflect.ValueOf(serviceStruct)
	s.serviceType = reflect.TypeOf(serviceStruct)
	s.name = reflect.Indirect(s.serviceStruct).Type().Name()
	s.method = make(map[string]*MethodType)
	if !ast.IsExported(s.name) {
		logrus.Fatal("invalid service: service can't be exported")
	}
	s.registerMethods()
	return s
}

func (s *service) registerMethods() {
	for i := 0; i < s.serviceType.NumMethod(); i++ {
		method := s.serviceType.Method(i)
		methodType := method.Type
		if methodType.NumIn() != 3 || methodType.NumOut() != 1 {
			continue
		}
		name := method.Name
		if !ast.IsExported(name) {
			continue
		}
		if methodType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		argType := methodType.In(1)
		replyType := methodType.In(2)
		if !isBuildInOrExported(argType) || !isBuildInOrExported(replyType) {
			continue
		}
		s.method[name] = &MethodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		logrus.Info("register method: ", name)
	}
}

func isBuildInOrExported(tp reflect.Type) bool {
	return ast.IsExported(tp.Name()) || tp.PkgPath() == ""
}

func (s *service) Call(mt *MethodType, arg, reply reflect.Value) error {
	atomic.AddUint64(&mt.numCalls, 1)
	fun := mt.method.Func
	value := fun.Call([]reflect.Value{s.serviceStruct, arg, reply})
	if value[0].Interface() != nil {
		return value[0].Interface().(error)
	}
	return nil
}
