package bifrost

import (
	"net"
	"os"
	"testing"
)

var np *Packet
var testAddr *net.UDPAddr
var testData []byte

func TestMain(m *testing.M) {
	testAddr, _ = net.ResolveUDPAddr("udp", "localhost:8000")
	testData = []byte("test")
	np = NewPacket(testAddr, testData)
	os.Exit(m.Run())
}

func TestNewPacket(t *testing.T) {

	if np == nil {
		t.Fatalf("Expected pointer to Packet, got nil")
	}

}

func TestSender(t *testing.T) {
	sender := np.sender
	if sender == nil {
		t.Fatalf("Expected pointer to UDPAddr, got nil")
	}
}

func BenchmarkNewPacket(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewPacket(testAddr, testData)
	}
}

func BenchmarkSender(b *testing.B) {
	for i := 0; i < b.N; i++ {
		np.Sender()
	}
}
