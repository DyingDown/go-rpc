package xclient

import (
	"context"
	. "go-rpc"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	option  *Option
	lock    sync.Mutex
	Clients map[string]*Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, option *Option) *XClient {
	return &XClient{
		d:       d,
		mode:    mode,
		option:  option,
		Clients: make(map[string]*Client),
	}
}

func (xclient *XClient) Close() error {
	xclient.lock.Lock()
	defer xclient.lock.Unlock()
	for k, v := range xclient.Clients {
		v.Close()
		delete(xclient.Clients, k)
	}
	return nil
}

func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.lock.Lock()
	defer xc.lock.Unlock()
	client, ok := xc.Clients[rpcAddr]
	if ok && !client.IsOnWork() {
		client.Close()
		delete(xc.Clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = XDial(rpcAddr, xc.option)
		if err != nil {
			return nil, err
		}
		xc.Clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, cntxt context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(cntxt, serviceMethod, args, reply)
}

func (xc *XClient) Call(cntxt context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, cntxt, serviceMethod, args, reply)
}

func (xc *XClient) Broadcast(cntxt context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var e error
	replyDone := reply == nil
	cntxt, cancel := context.WithCancel(cntxt)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var cpReply interface{}
			if reply != nil {
				cpReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, cntxt, serviceMethod, args, cpReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel()
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(cpReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}
