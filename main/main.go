package main

import (
	gorpc "go-rpc"
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

func (f *Foo) Sum(arg Args, reply *int) error {
	*reply = arg.A + arg.B
	return nil
}

func StartServer(addr chan string) {
	var foo Foo
	if err := gorpc.Register(foo); err != nil {
		logrus.Fatal("fail to register, %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		logrus.Fatalf("net work error: %v", err)
	}
	logrus.Infof("server start at %v", listener.Addr())
	addr <- listener.Addr().String()
	gorpc.Accept(listener)
}

func main() {
	addr := make(chan string)
	go StartServer(addr)
	client, err := gorpc.Dial("tcp", <-addr)
	if err != nil {
		logrus.Fatal("fail to dial, error: %v", err)
	}
	defer client.Close()
	time.Sleep(time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{i, i * i}
			reply := new(int)
			err := client.Call("Foo.Sum", args, reply)
			if err != nil {
				logrus.Fatal("fail to call Foo.Sum, error: %v", err)
			}
			logrus.Infof("%v + %v = %v", args.A, args.B, *reply)
		}(i)
	}
	wg.Wait()
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
