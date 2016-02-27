// Package bifrost is a UDP networking library meant for games
package bifrost

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/emef/bitfield"
)

// Connection tracks an individual IP:Port combination
type connection struct {
	addr           *net.UDPAddr
	lastHeard      time.Time
	localSequence  []byte
	remoteSequence []byte
	unackedPackets []*unackedPacketWrapper
	receivedQueue  []*Packet
	lastSent       time.Time
	socket         *Socket
}

// NewConnection creates a new Connection
func newConnection(a *net.UDPAddr, s *Socket) *connection {
	nc := connection{}
	nc.addr = a
	nc.lastHeard = time.Now()
	nc.localSequence = make([]byte, 4)
	nc.remoteSequence = make([]byte, 4)
	nc.unackedPackets = make([]*unackedPacketWrapper, 0, 1024)
	nc.receivedQueue = make([]*Packet, 0, 100)
	nc.socket = s
	binary.LittleEndian.PutUint32(nc.localSequence, 0)
	binary.LittleEndian.PutUint32(nc.remoteSequence, 0)
	go nc.sendKeepAlive()
	go nc.detectLostPackets()
	return &nc
}

func (c *connection) sendKeepAlive() {
	ticker := time.NewTicker(33 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			duration := time.Since(c.lastSent)
			if duration.Seconds() > 0.033 {
				np := NewPacket(c.addr, []byte("kpal"))
				c.socket.Outbound <- np
			}
		}
	}
}

func (c *connection) detectLostPackets() {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			log.Printf("Starting lost packet sweep")
			for _, u := range c.unackedPackets {
				if u != nil {

					duration := time.Since(u.created)
					if duration.Seconds() >= 1 {
						ne := NewEvent(2, c, u.p)
						c.delUnacked(u.p.SequenceInt())
						c.socket.Events <- ne
					}
				}
			}
		}
	}
}

func (c *connection) updateLastHeard() {
	c.lastHeard = time.Now()
}

func (c *connection) updateLastSent() {
	c.lastSent = time.Now()
}

func (c *connection) incrementLocalSequence() {
	curSeq := binary.LittleEndian.Uint32(c.localSequence)
	curSeq++
	binary.LittleEndian.PutUint32(c.localSequence, curSeq)
}

func (c *connection) incrementRemoteSequence() {
	curSeq := binary.LittleEndian.Uint32(c.remoteSequence)
	curSeq++
	binary.LittleEndian.PutUint32(c.remoteSequence, curSeq)
}

func (c *connection) lastHeardSeconds() time.Duration {
	return time.Since(c.lastHeard)
}

func (c *connection) key() string {
	var connKey string
	connKey = fmt.Sprintf("%s:%d", c.addr.IP, c.addr.Port)
	return connKey
}

func (c *connection) addUnacked(p *Packet) {
	nu := newUPW(p)
	c.unackedPackets = append(c.unackedPackets, nu)
}

func (c *connection) delUnacked(seq uint32) bool {
	for i, v := range c.unackedPackets {
		if v != nil {
			if v.p.SequenceInt() == seq {
				c.unackedPackets[i] = c.unackedPackets[len(c.unackedPackets)-1]
				c.unackedPackets = c.unackedPackets[:len(c.unackedPackets)-1]
			}
		}
	}

	return true
}

func (c *connection) addReceived(p *Packet) {

	rCount := c.CountReceivedQueue()
	if rCount > 32 {
		c.receivedQueue = c.receivedQueue[1:]
	}
	c.receivedQueue = append(c.receivedQueue, p)
}

func (c *connection) delReceived(p *Packet) bool {
	var toDelete *int
	for i, v := range c.receivedQueue {
		if v == p {
			toDelete = &i
			break
		}
	}
	if toDelete == nil {
		return false
	}
	c.receivedQueue = append(c.receivedQueue[:*toDelete],
		c.receivedQueue[*toDelete+1:]...)
	return true
}

func (c *connection) processAck(seq []byte) {

	pSeq := binary.LittleEndian.Uint32(seq)
	//log.Printf("RECV: Processing ack %d", pSeq)
	res, j := c.CheckSeqInUnacked(pSeq)
	if res == true {
		c.delUnacked(uint32(j))
	} else {

	}
}

func (c *connection) processAcks(a *bitfield.BitField, seq []byte) {
	//log.Printf("RECV: Processing acks")
	count := binary.LittleEndian.Uint32(seq)
	var i uint32
	for i = 1; i < 32; i++ {
		seqCheck := count - i
		res, j := c.CheckSeqInUnacked(seqCheck)
		if res == true {
			c.delUnacked(uint32(j))
		} else {

		}
	}
}

func (c *connection) composeAcks() bitfield.BitField {
	acks := bitfield.New(32)
	count := binary.LittleEndian.Uint32(c.remoteSequence)

	var i uint32
	for i = 1; i <= 32; i++ {
		seqCheck := count - i
		if c.CheckSeqInReceived(seqCheck) == true {

			acks.Set(i - 1)
		}
	}
	return acks
}

func (c *connection) CheckSeqInReceived(s uint32) bool {
	for _, v := range c.receivedQueue {
		if v.SequenceInt() == s {
			return true
		}
	}
	return false
}

func (c *connection) CheckSeqInUnacked(s uint32) (bool, int) {
	for i, v := range c.unackedPackets {
		if v.p.SequenceInt() == s {
			//log.Printf("RECV: Found %d in unacked", s)
			return true, i
		}
	}
	return false, 0
}
func (c *connection) CountReceivedQueue() int {
	counter := 0
	for _, v := range c.receivedQueue {
		if v != nil {
			counter++
		}
	}
	return counter
}

func (c *connection) PrintReceivedQueue() {
	//log.Printf("RQ is:")
	for _, v := range c.receivedQueue {
		if v != nil {
			log.Printf("%d %p", v.SequenceInt(), v)
		}
	}
}

func (c *connection) RemoteSequenceInt() uint32 {
	return binary.LittleEndian.Uint32(c.remoteSequence)
}

func (c *connection) LocalSequenceInt() uint32 {
	return binary.LittleEndian.Uint32(c.localSequence)
}

func (c *connection) PrintUnacked() {
	//var buf bytes.Buffer
	for _, v := range c.unackedPackets {
		if v != nil {
			log.Printf("Unacked: %d", v.p.SequenceInt())

		}
	}
}
