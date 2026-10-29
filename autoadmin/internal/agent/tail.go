package agent

import (
	"fmt"
	"sync"

	"autoadmin/internal/agent/pb"
)

type TailConnection struct {
	sess   *session
	id     string
	Events <-chan *pb.TailEvent
	once   sync.Once
}

func (gateway *Gateway) OpenTail(instance, id, path string) (*TailConnection, error) {
	if gateway == nil {
		return nil, ErrAgentOffline
	}
	gateway.mu.RLock()
	sess := gateway.sessions[instance]
	gateway.mu.RUnlock()
	if sess == nil {
		return nil, ErrAgentOffline
	}
	ch := make(chan *pb.TailEvent, 8)
	sess.mu.Lock()
	sess.tailEvents[id] = ch
	sess.mu.Unlock()
	conn := &TailConnection{sess: sess, id: id, Events: ch}
	if err := conn.Command("prepare", path, ""); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Pin commands to the original session: a reconnect must never resume a reader
// or send its stop command to a replacement agent session.
func (conn *TailConnection) Command(action, path, identity string) error {
	select {
	case conn.sess.tailControl <- &pb.TailRequest{RequestId: conn.id, Action: action, Path: path, Identity: identity}:
		return nil
	default:
		return fmt.Errorf("agent live-tail control queue full")
	}
}

func (conn *TailConnection) Close() {
	conn.once.Do(func() {
		_ = conn.Command("stop", "", "")
		conn.sess.mu.Lock()
		if ch := conn.sess.tailEvents[conn.id]; ch != nil {
			delete(conn.sess.tailEvents, conn.id)
			close(ch)
		}
		conn.sess.mu.Unlock()
	})
}

func (sess *session) sendTailEvent(event *pb.TailEvent) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if ch := sess.tailEvents[event.RequestId]; ch != nil {
		if len(event.Data) > 32768 || len(event.Identity) > 128 || len(event.Message) > 8192 {
			delete(sess.tailEvents, event.RequestId)
			close(ch)
			return
		}
		// Never wait for a browser in the shared gRPC receive loop. Closing the
		// bounded queue terminates this viewer; its agent lease bounds cleanup.
		select {
		case ch <- event:
		default:
			delete(sess.tailEvents, event.RequestId)
			close(ch)
		}
	}
}
