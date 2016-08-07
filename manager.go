package bifrost

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type connectionManager struct {
	connectionsLock *sync.Mutex
	connections     map[string]*Connection
	socket          *Socket
	reaper          *time.Ticker
	control         chan bool
}

func newConnectionManager(s *Socket) *connectionManager {
	newConnectionManager := connectionManager{}
	newConnectionManager.connections = make(map[string]*Connection)
	newConnectionManager.socket = s
	newConnectionManager.reaper = time.NewTicker(time.Second * 5)
	newConnectionManager.control = make(chan bool, 1)
	newConnectionManager.connectionsLock = &sync.Mutex{}
	//go newConnectionManager.disconnected()
	return &newConnectionManager
}

func (cm *connectionManager) add(c *Connection) bool {
	cm.connectionsLock.Lock()
	defer cm.connectionsLock.Unlock()
	connKey := c.key()
	_, ok := cm.connections[connKey]
	if ok == false {
		cm.connections[connKey] = c

		return true
	}
	return false
}

func (cm *connectionManager) remove(c *Connection) bool {
	cm.connectionsLock.Lock()
	defer cm.connectionsLock.Unlock()
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
			cm.connectionsLock.Lock()

			for _, v := range cm.connections {
				if v.lastHeardSeconds() >= cm.socket.timeout {
					ne := NewEvent(1, v, nil)
					cm.socket.Events <- ne
					cm.remove(v)
				}
			}
			cm.connectionsLock.Unlock()
		}
	}
}

func (cm *connectionManager) find(r *net.UDPAddr) *Connection {
	cm.connectionsLock.Lock()
	defer cm.connectionsLock.Unlock()
	key := fmt.Sprintf("%s:%d", r.IP, r.Port)
	for k, v := range cm.connections {
		if key == k {
			return v
		}
	}
	return nil
}
