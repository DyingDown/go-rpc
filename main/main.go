package main

import (
	"encoding/json"
	"fmt"
	gorpc "go-rpc"
	"go-rpc/codec"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

func StartServer(addr chan string) {
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

	conn, err := net.Dial("tcp", <-addr)
	if err != nil {
		logrus.Fatal("fail to dial, error: %v", err)
	}
	defer conn.Close()
	time.Sleep(time.Second)
	json.NewEncoder(conn).Encode(gorpc.DefaultOption)
	cc := codec.NewGobCodec(conn)
	for i := 0; i < 5; i++ {
		header := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		cc.Write(header, fmt.Sprintf("%v", header.Seq))
		cc.ReadHeader(header)
		var reply string
		cc.ReadBody(reply)
		logrus.Infof("reply: %v", reply)
	}
}
