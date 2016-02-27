package main

import (
	"bifrost"
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"net"
	"sync"
	"time"
)

var results chan bool

func main() {
	var localAddress = flag.String("laddr", "", "local address to bind to")
	var remoteAddress = flag.String("raddr", "", "remote address to connect to")
	var localPort = flag.Int("lport", 0, "local port to bind to")
	var test = flag.String("test", "onepacketecho", "which test to run")
	flag.Parse()
	sSocket := bifrost.NewSocket(*localAddress, *remoteAddress, *localPort, 1024, []byte("mana"), 30)
	log.Printf("Bifrost Socket created!")
	wg := &sync.WaitGroup{}
	sSocket.Start(wg)
	results := make(chan bool, 10)
	go processSocketEvents(sSocket)
	switch *test {
	case "onepacketecho":
		log.Printf("Beginning OnePacketEcho test")
		go processTestOnePacketEcho(wg, sSocket, results)
		time.Sleep(2 * time.Second)
		log.Printf("Sending test packet")
		np := sendTestEchoPacket(sSocket)
		sSocket.Outbound <- np
		r := <-results
		if r == true {
			log.Printf("OnePacketEcho passed!")
			sSocket.Stop()
			log.Printf("Channels closed")
		} else {
			panic("OnePacketEcho failed!")
		}
	case "tenpacketecho":
		log.Printf("Beginning TenPacketEcho test")
		go processTestTenPacketEcho(wg, sSocket, results)
		time.Sleep(2 * time.Second)
		log.Printf("Sending test packets")
		np := sendStockEchoPacket(sSocket)
		sSocket.Outbound <- np
		r := <-results
		if r == true {
			log.Printf("TenPacketEcho passed!")
			sSocket.Stop()
			log.Printf("Channels closed")
		} else {
			panic("TenPacketEcho failed!")
		}
	case "thirtyfourpacketecho":
		log.Printf("Beginning ThirtyFourPacketEcho test")
		go processTestThirtyFourPacketEcho(wg, sSocket, results)
		time.Sleep(2 * time.Second)
		log.Printf("Sending test packets")
		np := sendStockEchoPacket(sSocket)
		sSocket.Outbound <- np
		r := <-results
		if r == true {
			log.Printf("ThirtyFourPacketEcho passed!")
			sSocket.Stop()
			log.Printf("Channels closed")
		} else {
			panic("ThirtyFourPacketEcho failed!")
		}
	case "keepalive":
		log.Printf("Beginning KeepAlive test")
		go processTestKeepAlives(wg, sSocket, results)
		time.Sleep(2 * time.Second)
		np := sendStockEchoPacket(sSocket)
		sSocket.Outbound <- np
		r := <-results
		if r == true {
			log.Printf("KeepAlive passed!")
			sSocket.Stop()
			log.Printf("Channels closed")
		} else {
			panic("KeepAlive failed!")
		}
	}
	wg.Wait()

}

func processTestKeepAlives(wg *sync.WaitGroup, s *bifrost.Socket, results chan bool) bool {
	log.Printf("Beginning keepalive test")
	wg.Add(1)
	defer wg.Done()
	//echoComp := []byte("echo")
	for {
		p := <-s.Inbound
		if p.SequenceInt() > 10000 {

		}
		//seq := binary.LittleEndian.Uint32(p.Sequence())
		//resp := sendStockEchoPacket(s)
		//s.Outbound <- resp
	}
}

func processTestThirtyFourPacketEcho(wg *sync.WaitGroup, s *bifrost.Socket, results chan bool) bool {
	log.Printf("Beginning processTestThirtyFourPacketEcho function")
	wg.Add(1)
	defer wg.Done()
	echoComp := []byte("echo")

	for {
		p := <-s.Inbound
		seq := binary.LittleEndian.Uint32(p.Sequence())
		if bytes.Equal(echoComp, *p.Payload()) {
			if seq > 35 {
				panic("Received packet with unexpected sequence number")
			} else if seq == 35 {
				log.Printf("Ack is %d and Acks for seq 35 are: %s", p.AckInt(), p.PrintAcks())
				results <- true
				return true

			} else if seq == 33 {
				log.Printf("Acks are: %s", p.PrintAcks())

			} else {
				log.Printf("Processing seq %d", p.SequenceInt())
			}
			resp := sendStockEchoPacket(s)
			s.Outbound <- resp
		} else {
			panic("Received non-echo packet from server")
		}
	}
}

// processTestOnePacketEcho Sends one packet to the server and checks to make
// sure the reply is valid
func processTestOnePacketEcho(wg *sync.WaitGroup, s *bifrost.Socket, results chan bool) bool {
	log.Printf("Beginning processTestOnePacketEcho function")
	wg.Add(1)
	defer wg.Done()
	echoComp := []byte("echo")

	for {
		p := <-s.Inbound
		seq := binary.LittleEndian.Uint32(p.Sequence())
		if bytes.Equal(echoComp, *p.Payload()) {
			if seq > 1 {
				panic("Received packet with unexpected sequence number")
			} else {
				// This should be a response to our initial echo Packet
				// It should have a seq # of 1
				if p.SequenceInt() != 1 {
					panic("Packet with invalid sequence number received from server")
				}
				// Ack field should be 0
				log.Printf("Ack 0 should be set: %d", p.AckInt())
				if p.AckInt() != 10 {
					panic("Ack in packet from server received")
				}
				results <- true
				return true
				//resp := sendEcho(p.Sender())
				//s.Outbound <- resp
			}
		} else {
			panic("Received non-echo packet from server")
		}
	}
}

func processTestTenPacketEcho(wg *sync.WaitGroup, s *bifrost.Socket, results chan bool) bool {
	log.Printf("Beginning processTestTenPacketEcho function")
	wg.Add(1)
	defer wg.Done()
	echoComp := []byte("echo")
	for {
		p := <-s.Inbound
		seq := binary.LittleEndian.Uint32(p.Sequence())
		if bytes.Equal(echoComp, *p.Payload()) {
			if seq > 10 {
				panic("Received packet with unexpected sequence number")
			} else if seq == 10 {
				results <- true
				return true
			} else {
				log.Printf("Ack is: %d and Acks are: %s", p.AckInt(), p.PrintAcks())
				if p.SequenceInt() == 2 {
					// We should have nothing in the bitfield set
					for x := 0; x < 32; x++ {
						if x == 0 && p.CheckAck(uint32(x)) == false {
							panic("Bitfield not correct for packet with sequence 2")
						} else if x > 0 && p.CheckAck(uint32(x)) == true {
							panic("Bitfield set when it shouldn't have been!")
						}
					}
				} else if p.SequenceInt() > 2 {
					for x := uint32(0); x < seq-1; x++ {
						if p.CheckAck(uint32(x)) == false {
							panic("Bitfield not set when it should have been!")
						}
					}
				} else {

				}
				resp := sendStockEchoPacket(s)
				s.Outbound <- resp
			}
		} else {
			panic("Received non-echo packet from server")
		}
	}
}

func sendTestEchoPacket(s *bifrost.Socket) *bifrost.Packet {
	np := bifrost.NewPacket(s.GetRemoteAddress(), []byte("echo"))
	np.SetSequence(10)
	return np
}

func sendStockEchoPacket(s *bifrost.Socket) *bifrost.Packet {
	np := bifrost.NewPacket(s.GetRemoteAddress(), []byte("echo"))
	return np
}

func sendKeepAlive(a *net.UDPAddr) *bifrost.Packet {
	np := bifrost.NewPacket(a, []byte("kpal"))
	return np
}

func processSocketEvents(s *bifrost.Socket) {
	log.Printf("Beginning processing of Socket events")
	for {
		e := <-s.Events
		msg := e.PrintEventMessage()
		log.Printf("Received %s socket event", msg)
	}
}
