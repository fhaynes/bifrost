package main

import (
	"bifrost"
	"bytes"
	"flag"
	"log"
	"net"
	"sync"
)

import _ "net/http/pprof"

func process(wg *sync.WaitGroup, s *bifrost.Socket) {
	log.Printf("Beginning process function")
	wg.Add(1)
	defer wg.Done()

	echoComp := []byte("echo")
	for {
		p := <-s.Inbound
		if bytes.Equal(echoComp, *p.Payload()) {
			//seq := binary.LittleEndian.Uint32(p.Sequence())
			resp := sendEcho(p.Sender())
			s.Outbound <- resp
		} else {

		}

	}
}
func main() {
	//go func() {
	//	log.Println(http.ListenAndServe("localhost:6060", nil))
	//}()
	log.Printf("Starting server")
	var localAddress = flag.String("laddr", "", "local address to bind to")
	var remoteAddress = flag.String("raddr", "", "remote address to connect to")
	var localPort = flag.Int("lport", 0, "local port to bind to")
	flag.Parse()
	log.Printf("Binding to %s on port %d. Connecting to %s", *localAddress, *localPort, *remoteAddress)
	sSocket := bifrost.NewSocket(*localAddress, *remoteAddress, *localPort, 1024, []byte("mana"), 30)
	log.Printf("Bifrost Socket created!")
	wg := &sync.WaitGroup{}
	sSocket.Start(wg)
	go process(wg, sSocket)
	go processSocketEvents(sSocket)
	wg.Wait()

}

func sendEcho(a *net.UDPAddr) *bifrost.Packet {
	np := bifrost.NewPacket(a, []byte("echo"))
	return np
}

func processSocketEvents(s *bifrost.Socket) {
	log.Printf("Beginning processing of Socket events")
	for {
		_ = <-s.Events
		//log.Printf("Received %s socket event", e.PrintEventMessage())
	}
}
