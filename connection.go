// Package bifrost is a UDP networking library meant for games
package bifrost

import (
	"bytes"
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
	addr               *net.UDPAddr
	lastHeard          time.Time
	lastHeardLock      *sync.Mutex
	localSequence      []byte
	remoteSequence     []byte
	remoteSequenceLock *sync.Mutex
	unackedPackets     []*unackedPacketWrapper
	unackedPacketsLock *sync.Mutex
	receivedQueue      []*Packet
	receivedQueueLock  *sync.Mutex
	lastSent           time.Time
	lastSentLock       *sync.Mutex
	socket             *Socket
	lastAckProcess     time.Time
}

// NewConnection creates a new Connection
func newConnection(a *net.UDPAddr, s *Socket) *connection {
	nc := connection{}
	nc.addr = a
	nc.lastHeard = time.Now()
	nc.lastHeardLock = &sync.Mutex{}
	nc.localSequence = make([]byte, 4)
	nc.remoteSequence = make([]byte, 4)

	nc.unackedPackets = make([]*unackedPacketWrapper, 0, 100)
	nc.unackedPacketsLock = &sync.Mutex{}
	nc.receivedQueue = make([]*Packet, 33, 33)
	nc.receivedQueueLock = &sync.Mutex{}
	nc.lastSentLock = &sync.Mutex{}
	nc.remoteSequenceLock = &sync.Mutex{}
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

func (c *connection) processAck(seq []byte) {

	pSeq := binary.LittleEndian.Uint32(seq)
	//log.Printf("RECV: Processing ack %d", pSeq)
	res, _ := c.CheckSeqInUnacked(pSeq)
	if res == true {
		c.delUnacked(pSeq)
	} else {

	}
}

func (c *connection) processAcks(a *bitfield.BitField, seq []byte) {
	count := binary.LittleEndian.Uint32(seq)
	var i uint32
	for i = 1; i < 32; i++ {
		seqCheck := count - i
		if a.Test(i - 1) {
			c.delUnacked(seqCheck)
		}
	}
	c.lastAckProcess = time.Now()
}

func (c *connection) composeAcks() bitfield.BitField {
	var buf bytes.Buffer
	acks := bitfield.New(32)
	c.remoteSequenceLock.Lock()
	count := binary.LittleEndian.Uint32(c.remoteSequence)
	c.remoteSequenceLock.Unlock()
	var i uint32
	for i = 0; i <= 32; i++ {
		seqCheck := count - i
		if seqCheck == 0 {
			return acks
		}
		if c.CheckSeqInReceived(seqCheck) == true {
			buf.WriteString(fmt.Sprintf("%d ", seqCheck))
			log.Printf("%d %d", seqCheck, i-1)
			acks.Set(i - 1)
		}

	}
	log.Printf("%s", &buf)
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
