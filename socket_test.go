package bifrost

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"sync"
	"testing"
	"time"
)

func TestNewSocket(t *testing.T) {
	ns := openTestSocket()
	defer ns.listenConn.Close()
	if ns == nil {
		t.Fatalf("Expected pointer to Socket, got nil")
	}
}

func TestNewSocketNoRemote(t *testing.T) {
	ns := NewSocket("localhost", "", 7000, 1024, []byte("mana"), 30)
	defer ns.listenConn.Close()
	if ns == nil {
		t.Fatalf("Expected pointer to Socket, got nil")
	}
	if ns.remoteAddr != nil {
		t.Fatalf("Expected remoteAddr to be empty string, but it was %s", ns.remoteAddr)
	}
}

func TestInbound(t *testing.T) {
	ns := openTestSocket()
	defer ns.listenConn.Close()
	wg := &sync.WaitGroup{}
	//ns.Start(wg)
	wg.Add(1)
	go process(wg, ns, t)
	client, _ := net.ResolveUDPAddr("udp", "localhost:7000")
	np := NewPacket(client, []byte("done"))
	ns.Inbound <- np
	//conn, _ := net.DialUDP("udp", nil, client)
	//conn.WriteToUDP([]byte("test"), client)ad
	wg.Wait()

}
func TestSendPacket(t *testing.T) {
	ns := openTestSocket()
	defer ns.listenConn.Close()
	wg := &sync.WaitGroup{}
	ns.Start(wg)
	go process(wg, ns, t)
	time.Sleep(2 * time.Second)
	np := composeTestPacket()
	ns.Outbound <- np
	wg.Wait()
	ns.listenConn.Close()

}

func TestRemoveAckedPacket(t *testing.T) {
	ns := openTestSocket()
	defer ns.listenConn.Close()
	wg := &sync.WaitGroup{}
	ns.Start(wg)
	go process(wg, ns, t)
	time.Sleep(2 * time.Second)
	for i := 0; i <= 32; i++ {
		np := composeTestPacket()
		ns.Outbound <- np
	}
}

func BenchmarkInbound(b *testing.B) {
	ns := openTestSocket()
	defer ns.listenConn.Close()
	wg := &sync.WaitGroup{}
	//ns.Start(wg)
	wg.Add(1)
	go bProcess(wg, ns, b)
	client, _ := net.ResolveUDPAddr("udp", "localhost:7000")

	for i := 0; i < b.N; i++ {
		np := NewPacket(client, []byte("test"))
		ns.Inbound <- np
	}
	np := NewPacket(client, []byte("done"))
	ns.Inbound <- np
	//conn, _ := net.DialUDP("udp", nil, client)
	//conn.WriteToUDP([]byte("test"), client)
	wg.Wait()
}

func process(wg *sync.WaitGroup, s *Socket, t *testing.T) {
	defer wg.Done()
	log.Printf("Starting process")
	doneComp := []byte("done")
	ackComp := []byte("acktest")
	for {
		p := <-s.Inbound

		if p == nil {
			t.Fatalf("Expected p to be Packet, got nil")
		}
		if bytes.Equal(p.payload, doneComp) {
			break
		} else if bytes.Equal(p.payload, ackComp) {
			// We should have one packet in the connection received queue
			res := p.c.countReceivedQueue()
			if res != 1 {
				t.Fatalf("Expected connection to have 1 packet in received queue")
			}
			response := composeTestPacket()
			binary.LittleEndian.PutUint32(response.sequence, 1)
			p.acks = p.c.composeAcks()
			log.Printf("response acks is: %d", p.acks)
			ackRes := p.acks.Test(0)
			if ackRes != true {
				t.Fatalf("Expected 0 bit in ack bitfield to be set, it is not")
			}
			s.Outbound <- response
			break
		} else {
			log.Printf("Done packet not received")
			continue
		}
	}
	log.Printf("Done processing packets")
	close(s.Outbound)
}

func bProcess(wg *sync.WaitGroup, s *Socket, b *testing.B) {
	defer wg.Done()
	comp := []byte("done")
	for {
		p := <-s.Inbound
		if p == nil {
			b.Fatalf("Expected p to be Packet, got nil")
		}
		if bytes.Equal(p.payload, comp) {
			break
		} else {
			continue
		}
	}
}

func openTestSocket() *Socket {
	ns := NewSocket("localhost", "localhost", 7000, 1024, []byte("mana"), 30)
	return ns
}

func composeTestPacket() *Packet {

	payload := make([]byte, 4)
	payload = []byte("done")
	addr, _ := net.ResolveUDPAddr("udp", "localhost:7000")
	p := NewPacket(addr, payload)
	binary.LittleEndian.PutUint32(p.ack, 0)
	binary.LittleEndian.PutUint32(p.protocolID, 0)
	binary.LittleEndian.PutUint32(p.sequence, 0)
	p.protocolID = []byte("mana")
	return p
}
