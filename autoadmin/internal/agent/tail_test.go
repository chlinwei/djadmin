package agent

import (
	"testing"

	"autoadmin/internal/agent/pb"
)

func TestTailReceiveOverflowDoesNotBlockOtherCommands(t *testing.T) {
	sess := &session{
		tailEvents:  map[string]chan *pb.TailEvent{"slow": make(chan *pb.TailEvent, 8)},
		tailControl: make(chan *pb.TailRequest, 128),
	}
	channel := sess.tailEvents["slow"]
	for i := 0; i < 9; i++ {
		sess.sendTailEvent(&pb.TailEvent{RequestId: "slow", Data: make([]byte, 32768)})
	}
	if len(sess.tailEvents) != 0 {
		t.Fatal("overflow did not retire slow reader")
	}
	count := 0
	for range channel {
		count++
	}
	if count != 8 {
		t.Fatalf("buffered %d frames, want exactly 8", count)
	}
	conn := &TailConnection{sess: sess, id: "slow"}
	conn.Close()
	conn.Close()
	if len(sess.tailControl) != 1 {
		t.Fatal("close did not issue exactly one stop")
	}
}
