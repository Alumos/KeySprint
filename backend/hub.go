package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	conn                   *websocket.Conn
	send                   chan []byte
	rooms                  map[int64]bool
	studentID, studentName string
}

type hub struct {
	mu      sync.RWMutex
	clients map[*client]bool
}

func newHub() *hub { return &hub{clients: make(map[*client]bool)} }

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 32), rooms: make(map[int64]bool)}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	go h.writeLoop(c)
	h.readLoop(c)
}

func (h *hub) readLoop(c *client) {
	defer func() { h.remove(c); c.conn.Close() }()
	c.conn.SetReadLimit(64 << 10)
	_ = c.conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	for {
		var msg struct {
			Type           string  `json:"type"`
			CompetitionID  int64   `json:"competitionId"`
			CompetitionIDs []int64 `json:"competitionIds"`
			StudentID      string  `json:"studentId"`
			StudentName    string  `json:"studentName"`
		}
		if err := c.conn.ReadJSON(&msg); err != nil {
			return
		}
		switch msg.Type {
		case "student:join":
			h.mu.Lock()
			c.rooms[msg.CompetitionID] = true
			c.studentID = msg.StudentID
			c.studentName = msg.StudentName
			h.mu.Unlock()
			h.broadcastParticipants(msg.CompetitionID)
		case "teacher:watch":
			h.mu.Lock()
			for _, id := range msg.CompetitionIDs {
				c.rooms[id] = true
			}
			h.mu.Unlock()
		}
	}
}

func (h *hub) writeLoop(c *client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if c.conn.WriteMessage(websocket.TextMessage, data) != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	rooms := make([]int64, 0, len(c.rooms))
	for id := range c.rooms {
		rooms = append(rooms, id)
	}
	delete(h.clients, c)
	close(c.send)
	h.mu.Unlock()
	for _, id := range rooms {
		h.broadcastParticipants(id)
	}
}

func (h *hub) broadcast(room int64, event string, payload any) {
	b, _ := json.Marshal(map[string]any{"type": event, "data": payload})
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.rooms[room] {
			select {
			case c.send <- b:
			default:
			}
		}
	}
}

func (h *hub) participants(room int64) []map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := map[string]bool{}
	out := []map[string]string{}
	for c := range h.clients {
		if c.rooms[room] && c.studentID != "" && !seen[c.studentID] {
			seen[c.studentID] = true
			out = append(out, map[string]string{"studentId": c.studentID, "studentName": c.studentName})
		}
	}
	return out
}

func (h *hub) broadcastParticipants(room int64) {
	h.broadcast(room, "participants:update", map[string]any{"competitionId": room, "participants": h.participants(room)})
}
