package bifrost

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"

	"github.com/emef/bitfield"
)

// Packet is a struct representing headers and data we want to send over the
// network
type Packet struct {
	sender       *net.UDPAddr
	protocolID   []byte
	sequence     []byte
	sequenceLock *sync.Mutex
	ack          []byte
	acks         bitfield.BitField
	payload      []byte
	C            *Connection
}

// NewPacket initializes a new packet
func NewPacket(c *Connection, p []byte) *Packet {
	newPacket := Packet{}
	newPacket.ack = make([]byte, 4)
	newPacket.protocolID = make([]byte, 4)
	newPacket.sequence = make([]byte, 4)
	newPacket.acks = bitfield.New(32)
	newPacket.payload = p
	newPacket.sequenceLock = &sync.Mutex{}
	newPacket.C = c
	return &newPacket
}

// Sender returns the sender of the packet
func (p *Packet) Sender() *Connection {
	return p.C
}

// Sequence ...
func (p *Packet) Sequence() []byte {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	return p.sequence
}

// SetSequence sets the sequence number of a packet
func (p *Packet) SetSequence(s uint32) {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	binary.LittleEndian.PutUint32(p.sequence, s)
}

// SequenceInt returns the sequence numer as an int
func (p *Packet) SequenceInt() uint32 {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	return binary.LittleEndian.Uint32(p.sequence)
}

// Payload returns the payload of the packet
func (p *Packet) Payload() *[]byte {
	return &p.payload
}

// AckInt returns the ack of the packet as an int
func (p *Packet) AckInt() uint32 {
	return binary.LittleEndian.Uint32(p.ack)
}

// Connection returns the connection associated with the packet
func (p *Packet) Connection() *Connection {
	return p.C
}

// PrintAcks prints the acks in a packet
func (p *Packet) PrintAcks() *bytes.Buffer {
	var buf bytes.Buffer
	for i := 0; i < 32; i++ {
		if p.acks.Test(uint32(i)) {
			buf.WriteString("1")
		} else {
			buf.WriteString("0")
		}
	}
	return &buf
}

// CheckAck ...
func (p *Packet) CheckAck(f uint32) bool {
	return p.acks.Test(f)
}
