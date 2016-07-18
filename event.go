package bifrost

import "time"

// EventType is a sample wrapper type around various socket events
type EventType int

const (
	// Disconnected this event means that a socket has disconnected
	Disconnected = iota
	// LostPacket this event means that a packet was lost
	LostPacket = iota
)

// Event is a struct that represents a socket-level event, such as a client
// disconnected or a packet being lost
type Event struct {
	eventID EventType
	conn    string
	p       *Packet
	time    time.Time
}

// NewEvent creates a new Event struct and returns its address
func NewEvent(eid EventType, c *Connection, p *Packet) *Event {
	ne := Event{}
	ne.eventID = eid
	ne.conn = c.Addr.String()
	ne.p = p
	return &ne
}

// PrintEventMessage returns a message describing the event
func (e *Event) PrintEventMessage() string {
	return ""
}

// Type returns the type of Event
func (e *Event) Type() EventType {
	return e.eventID
}

// Source returns the ip/port that generated the event
func (e *Event) Source() string {
	return e.conn
}
