package gorpc

import (
	"context"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestClient_DialTimeout(t *testing.T) {
	t.Parallel()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	f := func(conn net.Conn, option *Option) (client *Client, err error) {
		conn.Close()
		time.Sleep(time.Second * 2)
		return nil, nil
	}
	t.Run("timeout", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &Option{ConnectTimeout: time.Second})
		_assert(err != nil && strings.Contains(err.Error(), "connection time out"), "expect a timeout error")
	})
	t.Run("0", func(t *testing.T) {
		_, err := dialTimeout(f, "tcp", l.Addr().String(), &Option{ConnectTimeout: 0})
		_assert(err == nil, "no limit")
	})
}

type Bar int

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}

func StartServer(addr chan string) {
	var bar Bar
	Register(&bar)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr <- l.Addr().String()
	Accept(l)
}

func TestClient_Call(t *testing.T) {
	t.Parallel()
	addr := make(chan string)
	go StartServer(addr)
	ad := <-addr
	time.Sleep(time.Second)
	t.Run("client timeout", func(t *testing.T) {
		client, _ := Dial("tcp", ad)
		cntxt, _ := context.WithTimeout(context.Background(), time.Second)
		var reply int
		err := client.Call(cntxt, "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), cntxt.Err().Error()), "expect a timeout error")
	})
	t.Run("server handle timeout", func(t *testing.T) {
		client, _ := Dial("tcp", ad, &Option{HandleTimeOout: time.Second})
		var reply int
		err := client.Call(context.Background(), "Bar.Timeout", 1, &reply)
		_assert(err != nil && strings.Contains(err.Error(), "timeout"), "expect a timeout error")
	})
}

func TestXDial(t *testing.T) {
	if runtime.GOOS == "linux" {
		ch := make(chan struct{})
		addr := "/tmp/goprc.sock"
		go func() {
			os.Remove(addr)
			l, err := net.Listen("unix", addr)
			if err != nil {
				panic("fail to listen unix socket" + err.Error())
			}
			ch <- struct{}{}
			logrus.Info("xxxxx")
			Accept(l)
		}()
		<-ch
		_, err := XDial("unix@" + addr)
		_assert(err == nil, "fail to connect unix socket")
	}
}
