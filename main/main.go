package main

import (
	"context"
	gorpc "go-rpc"
	"go-rpc/xclient"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type Foo int

type Args struct {
	A int
	B int
}

func (f Foo) Sum(arg Args, reply *int) error {
	*reply = arg.A + arg.B
	return nil
}

func (f Foo) Sleep(arg Args, reply *int) error {
	time.Sleep(time.Duration(arg.A) * time.Second)
	*reply = arg.A + arg.B
	return nil
}

func StartServer(addr chan string) {
	var foo Foo
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	server := gorpc.NewServer()
	server.Register(&foo)
	addr <- listener.Addr().String()
	server.Accept(listener)
}

func foo(xc *xclient.XClient, cntxt context.Context, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	if typ == "call" {
		err = xc.Call(cntxt, serviceMethod, args, &reply)
	} else if typ == "broadcast" {
		err = xc.Broadcast(cntxt, serviceMethod, args, &reply)
	}
	if err != nil {
		logrus.Errorf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		logrus.Infof("%s %s success: %d + %d = %d", typ, serviceMethod, args.A, args.B, reply)
	}
}
func call(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xc.Close()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "call", "Foo.Sum", &Args{i, i * i})
		}(i)
	}
	wg.Wait()
}

func broadcast(addr1, addr2 string) {
	d := xclient.NewMultiServerDiscovery([]string{"tcp@" + addr1, "tcp@" + addr2})
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer xc.Close()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), "broadcast", "Foo.Sum", &Args{i, i * i})
			cntxt, _ := context.WithTimeout(context.Background(), time.Second*2)
			foo(xc, cntxt, "broadcast", "Foo.Sleep", &Args{i, i * i})
		}(i)
	}
	wg.Wait()
}
func main() {
	addr1 := make(chan string)
	addr2 := make(chan string)
	go StartServer(addr1)
	go StartServer(addr2)

	ad1 := <-addr1
	ad2 := <-addr2
	time.Sleep(time.Second)
	call(ad1, ad2)
	broadcast(ad1, ad2)
}

// 设置 log 输出格式
func init() {
	//设置output,默认为stderr,可以为任何io.Writer，比如文件*os.File
	log.SetOutput(os.Stdout)
	//设置最低loglevel
	log.SetLevel(log.InfoLevel)
	log.SetReportCaller(true)
	log.SetFormatter(&gorpc.MyFormatter{})
}
