package tui

import (
	"net"
	"testing"
	"time"
)

func TestUDPSubscriptionRequiresServerAcknowledgement(t *testing.T) {
	server, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	state := NewAppState(nil, nil, "", "", server.LocalAddr().String(), "")
	state.setSession("test-token", "test-user", "test-user")
	worker := NewUDPWorker(state)
	worker.Start()
	defer worker.Stop()

	if err := server.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 256)
	_, client, err := server.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if state.connected("udp") {
		t.Fatal("reported subscription before server acknowledgement")
	}
	ack := []byte(`{"type":"subscribed","status":"ok"}`)
	if _, err := server.WriteToUDP(ack, client); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !state.connected("udp") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !state.connected("udp") {
		t.Fatal("subscription was not confirmed by the server")
	}
	worker.Stop()
	if state.connected("udp") {
		t.Fatal("stopped worker still reports a subscription")
	}
}
