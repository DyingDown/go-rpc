package gorpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go-rpc/codec"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

type Client struct {
	cc       codec.Codec
	option   *Option
	sending  sync.Mutex
	header   codec.Header
	mu       sync.Mutex
	seq      uint64
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
}

type clientResult struct {
	client *Client
	err    error
}

type NewClientFunc func(conn net.Conn, option *Option) (client *Client, err error)

var _ io.Closer = (*Client)(nil)

var ShutDownErr = errors.New("connection is shut down")

func (call *Call) done() {
	call.Done <- call
}

func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return ShutDownErr
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client) IsOnWork() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.closing && !client.shutdown
}

func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ShutDownErr
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

func (client *Client) recieve() {
	var err error
	for err == nil {
		h := new(codec.Header)
		if err = client.cc.ReadHeader(h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		if call == nil {
			err = client.cc.ReadBody(nil)
		} else if h.Error != "" {
			call.Error = errors.New(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		} else {
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("fail to read body " + err.Error())
			}
			call.done()
		}
	}
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, option *Option) (*Client, error) {
	codecFunc := codec.NewCodecFuncMap[option.CodecType]
	if codecFunc == nil {
		err := errors.New("invalid codectype" + string(option.CodecType))
		logrus.Errorf("client error: %v", err)
		return nil, err
	}
	enc := json.NewEncoder(conn)
	err := enc.Encode(option)
	if err != nil {
		err := errors.New("fail to decode opion" + err.Error())
		logrus.Error(err)
		conn.Close()
		return nil, err
	}
	return newClientCodec(codecFunc(conn), option), nil
}

func newClientCodec(cc codec.Codec, option *Option) *Client {
	client := &Client{
		seq:     1,
		cc:      cc,
		option:  option,
		pending: make(map[uint64]*Call),
	}
	go client.recieve()
	return client
}

func parseOption(options ...*Option) (*Option, error) {
	if len(options) == 0 || options[0] == nil {
		return DefaultOption, nil
	}
	if len(options) > 1 {
		return nil, errors.New("option is more than one")
	}
	option := options[0]
	option.MagicNumber = DefaultOption.MagicNumber
	if option.CodecType == "" {
		option.CodecType = DefaultOption.CodecType
	}
	return option, nil
}

func Dial(network, addr string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, addr, opts...)
}

func dialTimeout(f NewClientFunc, network, address string, options ...*Option) (client *Client, error error) {
	option, err := parseOption(options...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, option.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err := f(conn, option)
		ch <- clientResult{client: client, err: err}
	}()
	if option.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(option.ConnectTimeout):
		logrus.Error("timeout")
		return nil, errors.New("client: connection time out, expected within" + option.ConnectTimeout.String())
	case result := <-ch:
		return result.client, result.err
	}
}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	id, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = id
	client.header.Error = ""

	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(id)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		logrus.Panic("client error: done channel has no capacity")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

func (client *Client) Call(contex context.Context, serviceMethod string, args, reply interface{}) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-contex.Done():
		return errors.New("client: call timeout: " + contex.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}

func NewHTTPClient(conn net.Conn, option *Option) (*Client, error) {
	io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", defaultRPCPath))
	respons, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "CONNECT"})
	if err == nil && respons.Status == connected {
		return NewClient(conn, option)
	}
	if err == nil {
		err = errors.New("unexpected http status: " + respons.Status)
	}
	return nil, err
}

func DialHTTP(network, addr string, option ...*Option) (*Client, error) {
	return dialTimeout(NewHTTPClient, network, addr, option...)
}

func XDial(rpcAddr string, options ...*Option) (*Client, error) {
	parts := strings.Split(rpcAddr, "@")
	if len(parts) != 2 {
		return nil, errors.New("invalid address formate: " + rpcAddr + ". expected protocal@address")
	}
	protocal := parts[0]
	addr := parts[1]
	if protocal == "http" {
		return DialHTTP("tcp", addr, options...)
	} else {
		return Dial(protocal, addr, options...)
	}
}
