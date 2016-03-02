// Package bifrost is a UDP networking library meant for games
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

// Connection tracks an individual IP:Port combination
type connection struct {
	rtt    float64
	rttMax float64
	maxSeq uint32
	addr   *net.UDPAddr

	lastHeard      time.Time
	lastAckProcess time.Time
	lastSent       time.Time

	localSequence  []byte
	remoteSequence []byte

	unackedPackets []*unackedPacketWrapper
	ackedPackets   []*Packet
	receivedQueue  []*Packet

	lastSentLock       *sync.Mutex
	receivedQueueLock  *sync.Mutex
	lastHeardLock      *sync.Mutex
	unackedPacketsLock *sync.Mutex
	remoteSequenceLock *sync.Mutex
	ackedPacketsLock   *sync.Mutex

	socket *Socket
}

// NewConnection creates a new Connection
func newConnection(a *net.UDPAddr, s *Socket) *connection {
	nc := connection{}
	nc.addr = a
	nc.rtt = 0.0
	nc.rttMax = 1.0
	nc.lastHeard = time.Now()
	nc.lastHeardLock = &sync.Mutex{}
	nc.localSequence = make([]byte, 4)
	nc.remoteSequence = make([]byte, 4)
	nc.maxSeq = 4294967295
	nc.unackedPackets = make([]*unackedPacketWrapper, 0, 100)
	nc.unackedPacketsLock = &sync.Mutex{}
	nc.receivedQueue = make([]*Packet, 33, 33)
	nc.receivedQueueLock = &sync.Mutex{}
	nc.lastSentLock = &sync.Mutex{}
	nc.remoteSequenceLock = &sync.Mutex{}
	nc.ackedPacketsLock = &sync.Mutex{}
	nc.socket = s
	binary.LittleEndian.PutUint32(nc.localSequence, 0)
	binary.LittleEndian.PutUint32(nc.remoteSequence, 0)
	go nc.sendKeepAlive()
	go nc.detectLostPackets()
	return &nc
}

func (c *connection) sendKeepAlive() {
	ticker := time.NewTicker(33 * time.Millisecond)
	for _ = range ticker.C {
		c.lastSentLock.Lock()
		duration := time.Since(c.lastSent)
		c.lastSentLock.Unlock()
		if duration.Seconds() > 0.033 {
			np := NewPacket(c.addr, []byte("kpal"))
			c.socket.Outbound <- np
		}
	}
}

func (c *connection) detectLostPackets() {
	ticker := time.NewTicker(1 * time.Second)

	for _ = range ticker.C {

		c.unackedPacketsLock.Lock()
		for _, u := range c.unackedPackets {
			if u != nil {
				c.lastHeardLock.Lock()
				duration := time.Since(u.created)
				c.lastHeardLock.Unlock()
				if duration.Seconds() >= 1 {
					log.Printf("Time since for packet %d is %s", u.p.SequenceInt(), duration)
					ne := NewEvent(2, c, u.p)
					c.unackedPacketsLock.Unlock()
					c.delUnacked(u.p.SequenceInt())
					c.unackedPacketsLock.Lock()
					c.socket.Events <- ne
				}
			}
		}
		c.unackedPacketsLock.Unlock()

	}
}

func (c *connection) updateLastHeard() {
	c.lastHeardLock.Lock()
	defer c.lastHeardLock.Unlock()
	c.lastHeard = time.Now()
}

func (c *connection) updateLastSent() {
	c.lastSentLock.Lock()
	defer c.lastSentLock.Unlock()
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
	c.lastHeardLock.Lock()
	defer c.lastHeardLock.Unlock()
	return time.Since(c.lastHeard)
}

func (c *connection) key() string {
	var connKey string
	connKey = fmt.Sprintf("%s:%d", c.addr.IP, c.addr.Port)
	return connKey
}

func (c *connection) addUnacked(p *Packet) {
	nu := newUPW(p)

	c.unackedPacketsLock.Lock()

	defer c.unackedPacketsLock.Unlock()
	c.unackedPackets = append(c.unackedPackets, nu)
	return
}

func (c *connection) delUnacked(seq uint32) bool {
	c.unackedPacketsLock.Lock()
	defer c.unackedPacketsLock.Unlock()
	for i, v := range c.unackedPackets {
		if v != nil {
			if v.p.SequenceInt() == seq {
				c.unackedPackets = append(c.unackedPackets[:i], c.unackedPackets[i+1:]...)
				return true
			}
		}
	}
	return false
}

func (c *connection) addAcked(p *Packet) {
	c.ackedPacketsLock.Lock()
	defer c.ackedPacketsLock.Unlock()
	c.ackedPackets = append(c.ackedPackets, p)
	return
}

func (c *connection) addReceived(p *Packet) {
	c.receivedQueueLock.Lock()
	defer c.receivedQueueLock.Unlock()
	rCount := len(c.receivedQueue)
	if rCount > 32 {
		c.receivedQueue = c.receivedQueue[1:]
	}
	c.receivedQueue = append(c.receivedQueue, p)
}

func (c *connection) delReceived(p *Packet) bool {
	c.receivedQueueLock.Lock()
	defer c.receivedQueueLock.Unlock()
	for i, v := range c.receivedQueue {
		if v == p {
			c.receivedQueue = append(c.receivedQueue[:i], c.receivedQueue[i+1:]...)
			return true
		}
	}
	return false
}

func (c *connection) processAck(bAck []byte, a *bitfield.BitField) {
	ack := binary.LittleEndian.Uint32(bAck)
	c.unackedPacketsLock.Lock()
	defer c.unackedPacketsLock.Unlock()
	if len(c.unackedPackets) == 0 {
		return
	}
	for _, eachPacket := range c.unackedPackets {
		acked := false
		if eachPacket.p.SequenceInt() == ack {
			acked = true
		} else if !c.sequenceMoreRecent(eachPacket.p.SequenceInt(), ack, c.maxSeq) {
			bitIndex := c.bitIndexForSequence(eachPacket.p.SequenceInt(), ack, c.maxSeq)
			if bitIndex <= 31 {
				acked = a.Test(uint32(bitIndex))
			}
		}
		if acked == true {
			c.addAcked(eachPacket.p)
			c.unackedPacketsLock.Unlock()
			c.delUnacked(eachPacket.p.SequenceInt())
			c.unackedPacketsLock.Lock()
		}
	}
}

func (c *connection) sequenceMoreRecent(seq1 uint32, seq2 uint32, maxSeq uint32) bool {
	return (seq1 > seq2) && (seq1-seq2 <= maxSeq/2) || (seq2 > seq1) && (seq2-seq1 > maxSeq/2)
}

func (c *connection) bitIndexForSequence(seq uint32, ack uint32, maxSeq uint32) int {
	if seq > ack {
		return int(ack + (maxSeq - seq))
	}
	return int(ack - 1 - seq)
}

func (c *connection) composeAcks() bitfield.BitField {
	acks := bitfield.New(32)
	c.remoteSequenceLock.Lock()
	ack := binary.LittleEndian.Uint32(c.remoteSequence)
	c.remoteSequenceLock.Unlock()
	c.receivedQueueLock.Lock()
	for _, eachPacket := range c.receivedQueue {
		if eachPacket == nil {
			continue
		}
		if eachPacket.SequenceInt() == ack || c.sequenceMoreRecent(eachPacket.SequenceInt(), ack, c.maxSeq) {
			break
		}
		bitIndex := c.bitIndexForSequence(eachPacket.SequenceInt(), ack, c.maxSeq)
		if bitIndex <= 31 {
			acks.Set(uint32(bitIndex))
		}
	}
	c.receivedQueueLock.Unlock()
	return acks
}

func (c *connection) CheckSeqInReceived(s uint32) bool {
	c.receivedQueueLock.Lock()
	defer c.receivedQueueLock.Unlock()
	for _, v := range c.receivedQueue {
		if v != nil {
			if v.SequenceInt() == s {
				return true
			}
		}
	}
	return false
}

func (c *connection) CheckSeqInUnacked(s uint32) (bool, int) {
	c.unackedPacketsLock.Lock()
	defer c.unackedPacketsLock.Unlock()
	for i, v := range c.unackedPackets {
		if v.p.SequenceInt() == s {
			//log.Printf("RECV: Found %d in unacked", s)
			return true, i
		}
	}
	return false, 0
}
func (c *connection) CountReceivedQueue() int {
	c.receivedQueueLock.Lock()
	defer c.receivedQueueLock.Unlock()
	return len(c.receivedQueue)
}

func (c *connection) CountUnacked() int {
	c.unackedPacketsLock.Lock()
	defer c.unackedPacketsLock.Unlock()
	return len(c.unackedPackets)

}

func (c *connection) PrintReceivedQueue() {
	c.receivedQueueLock.Lock()
	defer c.receivedQueueLock.Unlock()
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

func (c *connection) SetLocalSequence(s uint32) {
	binary.LittleEndian.PutUint32(c.localSequence, s)
}

func (c *connection) PrintUnacked() {
	//var buf bytes.Buffer
	c.unackedPacketsLock.Lock()
	defer c.unackedPacketsLock.Unlock()
	for _, v := range c.unackedPackets {
		if v != nil {
			log.Printf("Unacked: %d", v.p.SequenceInt())

		}
	}
}
