package chatws

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultQueueSize = 64
	writeTimeout     = 5 * time.Second
)

type wsConn interface {
	SetWriteDeadline(t time.Time) error
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type websocketConn struct {
	raw *websocket.Conn
}

func (c *websocketConn) SetWriteDeadline(t time.Time) error { return c.raw.SetWriteDeadline(t) }
func (c *websocketConn) WriteMessage(messageType int, data []byte) error {
	return c.raw.WriteMessage(messageType, data)
}
func (c *websocketConn) Close() error { return c.raw.Close() }

type Client struct {
	conn   wsConn
	sendCh chan []byte
	done   chan struct{}
	closed atomic.Bool
}

type Hub struct {
	mu    sync.RWMutex
	users map[string]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{users: make(map[string]map[*Client]struct{})}
}

func (h *Hub) Add(userID string, conn *websocket.Conn) *Client {
	return h.addWithConn(userID, &websocketConn{raw: conn})
}

func (h *Hub) addWithConn(userID string, conn wsConn) *Client {
	c := &Client{
		conn:   conn,
		sendCh: make(chan []byte, defaultQueueSize),
		done:   make(chan struct{}),
	}
	h.mu.Lock()
	if h.users[userID] == nil {
		h.users[userID] = make(map[*Client]struct{})
	}
	h.users[userID][c] = struct{}{}
	h.mu.Unlock()
	go c.writeLoop()
	return c
}

func (h *Hub) Remove(userID string, c *Client) {
	if c == nil {
		return
	}
	h.mu.Lock()
	if set := h.users[userID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.users, userID)
		}
	}
	h.mu.Unlock()
	c.close()
}

func (h *Hub) Broadcast(userIDs []string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	targets := make([]*Client, 0, len(userIDs))
	for _, uid := range userIDs {
		for c := range h.users[uid] {
			targets = append(targets, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range targets {
		if c.closed.Load() {
			continue
		}
		select {
		case <-c.done:
			continue
		case c.sendCh <- b:
		default:
			c.close()
		}
	}
}

func (c *Client) SendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	select {
	case <-c.done:
		return nil
	case c.sendCh <- b:
	default:
		c.close()
	}
	return nil
}

func (c *Client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.sendCh:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *Client) close() {
	if c.closed.CompareAndSwap(false, true) {
		_ = c.conn.Close()
		close(c.done)
	}
}

