package bifrost

import (
	"fmt"
	"time"
)

// Event is a struct that represents a socket-level event, such as a client
// disconnected or a packet being lost
type Event struct {
	eventID int
	conn    *Connection
	p       *Packet
	time    time.Time
}

// NewEvent creates a new Event struct and returns its address
func NewEvent(eid int, c *Connection, p *Packet) *Event {
	ne := Event{}
	ne.eventID = eid
	ne.conn = c
	ne.p = p
	return &ne
}

// PrintEventMessage returns a message describing the event
func (e *Event) PrintEventMessage() string {

	if e.eventID == 1 {
		return fmt.Sprintf("%s disconnected", e.conn.Addr)
	} else if e.eventID == 2 {
		return fmt.Sprintf("Packet %d was lost", e.p.SequenceInt())
	} else {
		return fmt.Sprintf("Unknown eventID: %d", e.eventID)
	}
}
