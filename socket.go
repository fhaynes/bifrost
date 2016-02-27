// +build !debug

package bifrost

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/emef/bitfield"
)

// Socket is the core connectivity object that listens for packets and sends them
// out
type Socket struct {
	listenAddr  *net.UDPAddr
	listenConn  *net.UDPConn
	remoteAddr  *net.UDPAddr
	bufSize     int
	Inbound     chan *Packet
	Outbound    chan *Packet
	Events      chan *Event
	timeout     time.Duration
	pIdentifier []byte
	wg          *sync.WaitGroup
	cm          *connectionManager
}

// NewSocket creates and returns a new Socket
func NewSocket(ip string, remote string, port int, bufSize int, pIdentifier []byte, timeOut int) *Socket {
	log.Printf("Creating new socket")
	if remote == "" {
		log.Printf("No remote specified. This will be a listening socket only")
	}

	if len(ip) <= 0 {
		log.Fatal("Invalid string for IP passed to NewSocket")
	}

	if port == 0 {
		log.Fatal("Port must be greater than 0")
	}

	addrString := fmt.Sprintf("%s:%d", ip, port)
	listenAddr, err := net.ResolveUDPAddr("udp", addrString)

	if err != nil {
		log.Fatalf("Error trying to resolve UDP Address: %s. Error was: %s", addrString, err)
	}

	listenConn, err := net.ListenUDP("udp", listenAddr)

	if err != nil {
		log.Fatalf("Error trying to listen on socket: %s", err)
	}

	newSocket := Socket{}
	newSocket.listenAddr = listenAddr
	newSocket.listenConn = listenConn

	if remote != "" {
		remoteString := fmt.Sprintf("%s:%d", remote, port)
		remoteAddr, err := net.ResolveUDPAddr("udp", remoteString)
		newSocket.remoteAddr = remoteAddr
		if err != nil {
			log.Fatalf("Error trying to connect to remote server")
		}
	}

	newSocket.bufSize = bufSize
	newSocket.Inbound = make(chan *Packet, 1024)
	newSocket.Outbound = make(chan *Packet, 1024)
	newSocket.Events = make(chan *Event, 1024)
	newSocket.pIdentifier = pIdentifier
	newSocket.cm = newConnectionManager(&newSocket)
	timeoutString := fmt.Sprintf("%ds", timeOut)
	timeout, err := time.ParseDuration(timeoutString)
	newSocket.timeout = timeout
	return &newSocket

}

func (s *Socket) listen(wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("Starting to listen for packets on %s", s.listenAddr)

	for {
		buf := make([]byte, s.bufSize)
		n, addr, err := s.listenConn.ReadFromUDP(buf)
		//log.Printf("RECV: Processing new packet")
		newPacket := NewPacket(addr, buf)
		newPacket.sender = addr

		// Extract the various fields from the data received
		newPacket.protocolID = buf[0:4]
		newPacket.sequence = buf[4:8]
		if newPacket.SequenceInt() == 10 {
			continue
		}
		newPacket.ack = buf[8:12]
		newPacket.acks = bitfield.BitField(buf[12:16])
		newPacket.payload = buf[16:n]
		findResult := s.cm.find(addr)
		if findResult == nil {
			newConn := newConnection(addr, s)
			s.cm.add(newConn)
			findResult = newConn
		} else {
			findResult.updateLastHeard()
		}
		findResult.addReceived(newPacket)
		findResult.processAck(newPacket.ack)
		findResult.processAcks(&newPacket.acks, newPacket.sequence)
		log.Printf("Acks for packet %d are: %s", newPacket.SequenceInt(), newPacket.PrintAcks())

		newPacket.c = findResult
		//log.Printf("packet seq is %d and remote sequence is %d", newPacket.SequenceInt(), findResult.RemoteSequenceInt())
		seq1 := binary.LittleEndian.Uint32(newPacket.sequence)
		seq2 := binary.LittleEndian.Uint32(findResult.remoteSequence)

		if seq1 > seq2 {
			//log.Printf("Incrementing remote sequence")
			findResult.remoteSequence = newPacket.sequence
		}

		s.Inbound <- newPacket

		if err != nil {
			continue
		}
	}
}

func (s *Socket) send(wg *sync.WaitGroup) {
	defer wg.Done()
	for p := range s.Outbound {
		//log.Printf("SEND: New packet")
		c := s.cm.find(p.Sender())
		if c != nil {
			copy(p.sequence, c.localSequence)
			c.incrementLocalSequence()
			copy(p.ack, c.remoteSequence)
			p.acks = c.composeAcks()
			//log.Printf("New packet has seq %d", p.SequenceInt())
			c.addUnacked(p)
			c.updateLastSent()
		}
		//log.Printf("SEND: New packet seq is %d %p", p.SequenceInt(), p.sequence)
		var data []byte
		data = append(data, p.protocolID...)
		data = append(data, p.sequence...)
		data = append(data, p.ack...)
		data = append(data, p.acks...)
		data = append(data, p.payload...)
		//chance := rand.Float64()
		//if chance < 0.1 {
		//	log.Printf("Throwing away packet: %d", p.SequenceInt())
		//	continue
		//}
		_, err := s.listenConn.WriteToUDP(data, p.Sender())
		if err != nil {
			log.Printf("Error writing packet: %s", err)
			continue
		}
	}
}

// Start starts the Socket listening and sending
func (s *Socket) Start(wg *sync.WaitGroup) {
	go s.listen(wg)
	if s.remoteAddr != nil {
		wg.Add(1)
		go s.send(wg)
	}
}

// SetTimeout lets the user set the timeout for a Socket. If a Connection has
// not been heard from within that time, they are considered disconnected
func (s *Socket) SetTimeout(d time.Duration) bool {
	s.timeout = d
	return true
}

// SetProtocolID let's the user set a specific protocol ID to watch for in
// UDP packets. If it isn't present, the packet is ignored
func (s *Socket) SetProtocolID(pID []byte) bool {
	s.pIdentifier = pID
	return true
}

/*
func interfaces() {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Panicf("There was an error getting system interfaces")
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			log.Panicf("There was an error getting system interface addresses")
		}
		for _, addr := range addrs {

			switch v := addr.(type) {
			case *net.IPNet:
				ip := v.IP
			case *net.IPAddr:
				ip := v.IP

			}
		}
	}
}
*/
// GetRemoteAddress returns the remote address of the socket
func (s *Socket) GetRemoteAddress() *net.UDPAddr {
	return s.remoteAddr
}

// Stop shuts down a socket
func (s *Socket) Stop() {
	s.cm.control <- true
	close(s.Outbound)
	close(s.Inbound)
}