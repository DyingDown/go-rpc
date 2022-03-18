package gorpc

import (
	"encoding/json"
	"fmt"
	"go-rpc/codec"
	"io"
	"net"
	"reflect"
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
}

type Server struct{}

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
	var header = new(codec.Header)
	if err := cc.ReadHeader(header); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, err
		}
		logrus.Errorf("fail to read header: %v", err)
	}
	return header, nil
}

func (s *Server) readRequest(codec codec.Codec) (*Request, error) {
	header, err := s.readRequestHeader(codec)
	if err != nil {
		return nil, err
	}
	argv := reflect.New(reflect.TypeOf(""))
	if err := codec.ReadBody(argv.Interface()); err != nil {
		logrus.Errorf("fail to read body: %v", err)
	}
	return &Request{
		header:   header,
		argValue: argv,
	}, nil
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
	logrus.Info(request.header, request.argValue.Elem())
	request.replyValue = reflect.ValueOf(fmt.Sprintf("%s", request.header.Seq))
	s.sendResponse(codec, request.header, request.replyValue.Interface(), sending)
}
