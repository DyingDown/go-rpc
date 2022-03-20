package gorpc

import (
	"encoding/json"
	"errors"
	"go-rpc/codec"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

const MagicNumber = 0

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

type Request struct {
	header               *codec.Header
	argValue, replyValue reflect.Value
	mt                   *MethodType
	service              *service
}

type Server struct {
	serviceMap sync.Map
}

var invalidRequest = struct{}{}

func NewServer() *Server {
	return new(Server)
}

var DefaultServer = NewServer()

func (s *Server) Accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			logrus.Errorf("fail to accept rpc: %v", err)
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn io.ReadWriteCloser) {
	defer conn.Close()
	var option Option
	if err := json.NewDecoder(conn).Decode(&option); err != nil {
		logrus.Errorf("fail to decode option: %v", err)
		return
	}
	if option.MagicNumber != MagicNumber {
		logrus.Errorf("invalid magic number: %v", option.MagicNumber)
		return
	}
	codecfunc := codec.NewCodecFuncMap[option.CodecType]
	if codecfunc == nil {
		logrus.Errorf("invalid codec type: %v", option.CodecType)
		return
	}
	s.handleCodec(codecfunc(conn))
}

func Accept(listener net.Listener) {
	DefaultServer.Accept(listener)
}

func (s *Server) handleCodec(codec codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		request, err := s.readRequest(codec)
		if err != nil {
			if request == nil {
				break
			}
			request.header.Error = err.Error()
			s.sendResponse(codec, request.header, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go s.handleRequest(codec, request, sending, wg)
	}
	wg.Wait()
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var header codec.Header
	if err := cc.ReadHeader(&header); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, err
		}
		logrus.Errorf("fail to read header: %v", err)
	}
	return &header, nil
}

func (s *Server) readRequest(codec codec.Codec) (*Request, error) {
	header, err := s.readRequestHeader(codec)
	if err != nil {
		return nil, err
	}
	r := &Request{}
	r.header = header
	r.service, r.mt, err = s.FindService(header.ServiceMethod)
	if err != nil {
		return r, err
	}
	r.argValue = r.mt.newArgv()
	r.replyValue = r.mt.newReplyv()

	argV := r.argValue.Interface()
	if err := codec.ReadBody(argV); err != nil {
		logrus.Error("server: read request body err: %v", err)
		return r, err
	}
	return r, nil
}

func (s *Server) sendResponse(codec codec.Codec, header *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := codec.Write(header, body); err != nil {
		logrus.Errorf("fail to write response: %v", err)
	}
}

func (s *Server) handleRequest(codec codec.Codec, request *Request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := request.service.Call(request.mt, request.argValue, request.replyValue)
	if err != nil {
		request.header.Error = err.Error()
		s.sendResponse(codec, request.header, invalidRequest, sending)
		return
	}
	s.sendResponse(codec, request.header, request.replyValue.Interface(), sending)
}

func (s *Server) Register(serviceStruct interface{}) error {
	service := NewService(serviceStruct)
	if _, ok := s.serviceMap.LoadOrStore(service.name, service); ok {
		return errors.New("service all ready registered")
	}
	return nil
}

func Register(serviceStruct interface{}) error {
	return DefaultServer.Register(serviceStruct)
}

func (s *Server) FindService(serviceMethod string) (*service, *MethodType, error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		return nil, nil, errors.New("service method has illegal formate" + serviceMethod)
	}
	serviceName := serviceMethod[:dot]
	methodName := serviceMethod[dot+1:]
	serviceInter, ok := s.serviceMap.Load(serviceName)
	if !ok {
		return nil, nil, errors.New("server: service " + serviceName + " can't be found")
	}
	service := serviceInter.(*service)
	methodType := service.method[methodName]
	if methodType == nil {
		return nil, nil, errors.New("server: service method " + methodName + " can't be found")
	}
	return service, methodType, nil
}
