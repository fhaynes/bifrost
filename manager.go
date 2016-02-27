package bifrost

import (
	"fmt"
	"net"
	"time"
)

type connectionManager struct {
	connections map[string]*connection
	socket      *Socket
	reaper      *time.Ticker
	control     chan bool
}

func newConnectionManager(s *Socket) *connectionManager {
	newConnectionManager := connectionManager{}
	newConnectionManager.connections = make(map[string]*connection)
	newConnectionManager.socket = s
	newConnectionManager.reaper = time.NewTicker(time.Second * 5)
	newConnectionManager.control = make(chan bool, 1)
	go newConnectionManager.disconnected()
	return &newConnectionManager
}

func (cm *connectionManager) add(c *connection) bool {
	_, ok := cm.connections[c.key()]
	if ok == false {
		cm.connections[c.key()] = c
		return true
	}
	return false
}

func (cm *connectionManager) remove(c *connection) bool {
	_, ok := cm.connections[c.key()]
	if ok == true {
		delete(cm.connections, c.key())
		return true
	}
	return false
}

func (cm *connectionManager) disconnected() {
	for range cm.reaper.C {
		select {
		case <-cm.control:
			return
		default:
			for _, v := range cm.connections {
				if v.lastHeardSeconds() >= cm.socket.timeout {
					ne := NewEvent(1, v, nil)
					cm.socket.Events <- ne
					cm.remove(v)
				}
			}
		}
	}
}

func (cm *connectionManager) find(r *net.UDPAddr) *connection {
	key := fmt.Sprintf("%s:%d", r.IP, r.Port)
	for k, v := range cm.connections {
		if key == k {
			return v
		}
	}
	return nil
}
