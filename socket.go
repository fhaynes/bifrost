package bifrost

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
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
	packetloss  int
}

// NewSocket creates and returns a new Socket
func NewSocket(ip string, remote string, rPort int, bufSize int, pIdentifier []byte, timeOut int, lPort int) *Socket {
	log.Printf("Creating new socket")
	if remote == "" {
		log.Printf("No remote specified. This will be a listening socket only")
	}

	if len(ip) <= 0 {
		log.Fatal("Invalid string for IP passed to NewSocket")
	}

	if lPort == 0 {
		log.Fatal("lPort must be greater than 0")
	}

	addrString := fmt.Sprintf("%s:%d", ip, lPort)
	listenAddr, err := net.ResolveUDPAddr("udp", addrString)

	if err != nil {
		log.Fatalf("Error trying to resolve UDP Address: %s. Error was: %s", addrString, err)
	}

	listenConn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatalf("Error trying to listen: %s", err.Error())
	}
	listenConn.SetReadBuffer(1048576)

	if err != nil {
		log.Fatalf("Error trying to listen on socket: %s", err)
	}

	newSocket := Socket{}
	newSocket.listenAddr = listenAddr
	newSocket.listenConn = listenConn

	if remote != "" && rPort != 0 {
		remoteString := fmt.Sprintf("%s:%d", remote, rPort)
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
	newSocket.packetloss = 0
	timeoutString := fmt.Sprintf("%ds", timeOut)
	timeout, err := time.ParseDuration(timeoutString)
	newSocket.timeout = timeout
	return &newSocket

}

func (s *Socket) listen(wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, s.bufSize)
	for {
		_, addr, err := s.listenConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from socket: %s", err.Error())
			continue
		}
		//log.Printf("RECV: Processing new packet: %s", addr.String())
		newPacket := NewPacket(addr, nil)
		newPacket.sender = addr

		// Extract the various fields from the data received
		copy(newPacket.protocolID, buf[0:4])
		copy(newPacket.sequence, buf[4:8])
		copy(newPacket.ack, buf[8:12])
		newPacket.acks = bitfield.BitField(buf[12:16])
		newPacket.payload = make([]byte, len(buf))
		copy(newPacket.payload, buf[16:])
		findResult := s.cm.find(addr)
		if findResult == nil {

			newConn := newConnection(addr, s)
			s.cm.add(newConn)
			findResult = newConn
		} else {
			findResult.updateLastHeard()
		}
		findResult.addReceived(newPacket)
		if findResult.sequenceMoreRecent(binary.LittleEndian.Uint32(newPacket.sequence),
			binary.LittleEndian.Uint32(findResult.remoteSequence),
			findResult.maxSeq) {
			copy(findResult.remoteSequence, newPacket.sequence)

		}
		findResult.processAck(newPacket.ack, &newPacket.acks)
		//log.Printf("RECV: Ack/s for packet %d are: %d : %s", newPacket.SequenceInt(), newPacket.AckInt(), newPacket.PrintAcks())

		newPacket.c = findResult
		//log.Printf("packet seq is %d and remote sequence is %d", newPacket.SequenceInt(), findResult.RemoteSequenceInt())

		s.Inbound <- newPacket

		if err != nil {
			log.Printf("Listen socket error: %s", err.Error())
		}

	}
}

func (s *Socket) send(wg *sync.WaitGroup) {
	defer wg.Done()
	for p := range s.Outbound {
		//log.Printf("SEND: Sending packet to: %s", p.Sender())
		c := s.cm.find(p.Sender())
		if c == nil {
			newConn := newConnection(p.sender, s)
			s.cm.add(newConn)
			c = newConn
		}

		p.sequenceLock.Lock()
		copy(p.sequence, c.localSequence)
		p.sequenceLock.Unlock()
		c.incrementLocalSequence()

		c.remoteSequenceLock.Lock()
		copy(p.ack, c.remoteSequence)
		c.remoteSequenceLock.Unlock()

		c.addUnacked(p)

		p.acks = c.composeAcks()

		if c.LocalSequenceInt() > c.maxSeq {
			c.SetLocalSequence(uint32(0))
		}
		//log.Printf("New packet has seq %d", p.SequenceInt())

		c.updateLastSent()

		//log.Printf("SEND: New packet seq is %d %p", p.SequenceInt(), p.sequence)
		var data []byte
		data = append(data, p.protocolID...)
		data = append(data, p.sequence...)
		data = append(data, p.ack...)
		data = append(data, p.acks...)
		data = append(data, p.payload...)
		//		log.Printf("In send data is: %v payload is: %v", data, p.payload)
		//chance := rand.Float64()
		//if chance < 0.1 {
		//	log.Printf("Throwing away packet: %d", p.SequenceInt())
		//	continue
		//}
		//log.Printf("Sending to %s", p.Sender())
		//if math.Mod(float64(p.SequenceInt()), 100) == 0 {
		//log.Printf("SEND: Ack/s for packet %d are %d : %s", p.SequenceInt(), p.AckInt(), p.PrintAcks())
		//}
		if s.packetloss > 0 {
			randChance := rand.Int31n(100)
			if randChance <= int32(s.packetloss) {
				continue
			}
		}

		_, err := s.listenConn.WriteToUDP(data, p.Sender())
		if err != nil {
			log.Printf("Error writing packet: %s", err)
			continue
		}
		//log.Printf("Time for send was: %s", time.Since(sendStart))
	}
}

// Start starts the Socket listening and sending
func (s *Socket) Start(wg *sync.WaitGroup) {
	go s.listen(wg)
	wg.Add(1)
	go s.send(wg)
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

func (s *Socket) ListenConn() *net.UDPConn {
	return s.listenConn
}
