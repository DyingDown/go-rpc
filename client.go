package gorpc

import (
	"encoding/json"
	"errors"
	"go-rpc/codec"
	"io"
	"net"
	"sync"

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

func (call *Call) done() {
	call.Done <- call
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

var _ io.Closer = (*Client)(nil)

var ShutDownErr = errors.New("connection is shut down")

func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return ShutDownErr
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client) isOnWork() bool {
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
	option, err := parseOption(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			conn.Close()
		}
	}()
	return NewClient(conn, option)
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

func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
