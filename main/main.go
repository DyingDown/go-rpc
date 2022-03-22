package main

import (
	"context"
	"fmt"
	gorpc "go-rpc"
	"go-rpc/registry"
	"go-rpc/xclient"
	"net"
	"net/http"
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

func StartRegistry(ch chan string) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	registry.HandleHTTP()
	ch <- l.Addr().String()
	http.Serve(l, nil)
}

func StartServer(registryAddr string, wg *sync.WaitGroup) {
	var foo Foo
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	server := gorpc.NewServer()
	server.Register(&foo)
	registry.Hearbeat(registryAddr, "tcp@"+listener.Addr().String(), 0)
	wg.Done()
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

func call(registry string) {
	d := xclient.NewRegistryDiscovery(registry, 0)
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

func broadcast(registry string) {
	d := xclient.NewRegistryDiscovery(registry, 0)
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
	var wg sync.WaitGroup
	registryAddrCh := make(chan string)
	go StartRegistry(registryAddrCh)
	registryAddr := <-registryAddrCh
	registryAddr = fmt.Sprintf("http://%s%s", registryAddr, registry.DefaultPath)
	time.Sleep(time.Second)
	wg.Add(2)
	go StartServer(registryAddr, &wg)
	go StartServer(registryAddr, &wg)

	wg.Wait()

	time.Sleep(time.Second)
	call(registryAddr)
	broadcast(registryAddr)
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
