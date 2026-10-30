package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	conn        *websocket.Conn
	send        chan []byte
	rooms       map[int64]bool
	studentID   string
	studentName string
	teacher     bool
	// allowRoom is consulted before a teacher may subscribe to a competition.
	allowRoom func(int64) bool
}

type hub struct {
	mu      sync.RWMutex
	clients map[*client]bool
}

func newHub() *hub { return &hub{clients: make(map[*client]bool)} }

// Connection credentials (teacher ticket, student access code) are validated by
// app.websocket before the upgrade, including the Origin header. The upgrader
// therefore does not repeat the origin check.
var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// attach registers an authenticated connection and blocks in its read loop.
func (h *hub) attach(c *client) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	if !c.teacher {
		// Students are bound to their competition at upgrade time.
		for room := range c.rooms {
			h.broadcastParticipants(room)
		}
	}
	go h.writeLoop(c)
	h.readLoop(c)
}

func (h *hub) readLoop(c *client) {
	defer func() {
		h.remove(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(8 << 10)
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
		case "teacher:watch":
			if !c.teacher {
				continue
			}
			rooms := make(map[int64]bool, len(msg.CompetitionIDs))
			for _, id := range msg.CompetitionIDs {
				if id > 0 && (c.allowRoom == nil || c.allowRoom(id)) {
					rooms[id] = true
				}
			}
			h.setRooms(c, rooms)
		case "student:join":
			// A student socket is pinned to one competition at upgrade time. The
			// legacy message still refreshes the roster but cannot move rooms.
			if c.teacher || c.studentID == "" {
				continue
			}
			if msg.CompetitionID > 0 && c.rooms[msg.CompetitionID] {
				h.broadcastParticipants(msg.CompetitionID)
			}
		}
	}
}

func (h *hub) setRooms(c *client, rooms map[int64]bool) {
	h.mu.Lock()
	c.rooms = rooms
	h.mu.Unlock()
	for room := range rooms {
		h.broadcastParticipants(room)
	}
}

func (h *hub) writeLoop(c *client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-c.send:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if c.conn.WriteMessage(websocket.TextMessage, data) != nil {
				// Close the socket so the read loop unregisters this client now
				// instead of leaving a dead connection in every broadcast.
				_ = c.conn.Close()
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
				_ = c.conn.Close()
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

// broadcast reaches every client in the room (competition start/stop).
func (h *hub) broadcast(room int64, event string, payload any) {
	h.send(room, false, event, payload)
}

// broadcastTeachers reaches teacher consoles only. Rosters and result events
// carry student identity, so they are not fanned out to student sockets.
func (h *hub) broadcastTeachers(room int64, event string, payload any) {
	h.send(room, true, event, payload)
}

func (h *hub) send(room int64, teachersOnly bool, event string, payload any) {
	b, err := json.Marshal(map[string]any{"type": event, "data": payload})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if !c.rooms[room] || (teachersOnly && !c.teacher) {
			continue
		}
		select {
		case c.send <- b:
		default:
			// A slow client must not stall a broadcast; the next event or the
			// roster request will bring it back in sync.
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
	h.broadcastTeachers(room, "participants:update", map[string]any{
		"competitionId": room,
		"participants":  h.participants(room),
	})
}
