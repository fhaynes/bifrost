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
	c            *connection
}

// NewPacket initializes a new packet
func NewPacket(s *net.UDPAddr, p []byte) *Packet {
	newPacket := Packet{}
	newPacket.ack = make([]byte, 4)
	newPacket.protocolID = make([]byte, 4)
	newPacket.sequence = make([]byte, 4)
	newPacket.acks = bitfield.New(32)
	newPacket.sender = s
	newPacket.payload = p
	newPacket.sequenceLock = &sync.Mutex{}
	return &newPacket
}

// Sender returns the sender of the packet
func (p *Packet) Sender() *net.UDPAddr {
	return p.sender
}

func (p *Packet) Sequence() []byte {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	return p.sequence
}

func (p *Packet) SetSequence(s uint32) {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	binary.LittleEndian.PutUint32(p.sequence, s)
}

func (p *Packet) SequenceInt() uint32 {
	p.sequenceLock.Lock()
	defer p.sequenceLock.Unlock()
	return binary.LittleEndian.Uint32(p.sequence)
}

func (p *Packet) Payload() *[]byte {
	return &p.payload
}

func (p *Packet) AckInt() uint32 {
	return binary.LittleEndian.Uint32(p.ack)
}

func (p *Packet) Connection() *connection {
	return p.c
}

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

func (p *Packet) CheckAck(f uint32) bool {
	return p.acks.Test(f)
}
