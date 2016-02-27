package bifrost

import "time"

type unackedPacketWrapper struct {
	created time.Time
	p       *Packet
}

func newUPW(p *Packet) *unackedPacketWrapper {
	nu := unackedPacketWrapper{}
	nu.created = time.Now()
	nu.p = p
	return &nu
}
