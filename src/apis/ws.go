package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
	"github.com/gorilla/websocket"
)

type WebSocketHub struct {
	clients map[string]map[string]*websocket.Conn
	mu sync.Mutex
	expectedClientCounts map[string]int
	OnClientCountChangeMap map[string]func(count int)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for testing; tighten in production
		return true
	},
}

func NewWebSocketHub() *WebSocketHub{
	return &WebSocketHub{
		clients:make(map[string]map[string]*websocket.Conn),
		expectedClientCounts:make(map[string]int),
		OnClientCountChangeMap:make(map[string]func(int)),
	}
}

func (hub *WebSocketHub) AddClient(tournamentId string, clientId string, conn *websocket.Conn) {
	hub.mu.Lock()
	if hub.clients[tournamentId] == nil {
		hub.clients[tournamentId] = make(map[string]*websocket.Conn)
	}

	if oldConn, ok := hub.clients[tournamentId][clientId]; ok {
		fmt.Println("OldConn exists for tournamentId", tournamentId, "clientId", clientId)
		
		// Don't close immediately — just unblock ReadMessage
		go func(c *websocket.Conn) {
			_ = c.SetReadDeadline(time.Now())
		}(oldConn)
	}

	hub.clients[tournamentId][clientId] = conn
	hub.mu.Unlock()

	fmt.Println("Client", clientId, "added to tournament", tournamentId)

	if f, ok := hub.OnClientCountChangeMap[tournamentId]; ok {
		fmt.Println("OnClientCountChangeMap:", len(hub.clients[tournamentId]))
		f(len(hub.clients[tournamentId]))
	}
}

func (hub * WebSocketHub) RemoveClient(tournamentId string,clientId string){
	hub.mu.Lock()
	
	if hub.clients[tournamentId] != nil {
		if conn,ok := hub.clients[tournamentId][clientId]; ok{
			conn.Close()
			delete(hub.clients[tournamentId],clientId)
			hub.mu.Unlock()
			fmt.Println("Client", clientId, "removed from tournament", tournamentId)
			if f,ok := hub.OnClientCountChangeMap[tournamentId]; ok {
				f(len(hub.clients[tournamentId]))
			}
		}
	}
}

//Problem: During refresh, we first need to remove the existing connection through our goroutine
//After Removing the connection, we need to accept the new connection 
//The problem is that our frontend is sending multiple connection requests for some reason (need to fix on frontend)
//Our backend should be able to handle this though.

func (h *Handler) handleConnection(tournamentId, clientId string, conn *websocket.Conn) {
	defer func() {
		fmt.Printf("Cleaning up client %s from tournament %s\n", clientId, tournamentId)
		h.WebSocketHub.RemoveClient(tournamentId, clientId)
		conn.Close() // only called here
	}()

	// Optional: keepalive
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				fmt.Printf("Client %s disconnected normally: %v\n", clientId, err)
			} else if err.Error() == "use of closed network connection" {
				// This should no longer show up if we fix the race
				fmt.Printf("Client %s disconnected: %v\n", clientId, err)
			} else {
				fmt.Printf("Unexpected close for client %s: %v\n", clientId, err)
			}
			break
		}
	}
}


func (hub *WebSocketHub) Broadcast(tournamentId string, msg interface{}){
	hub.mu.Lock()
	defer hub.mu.Unlock()

	clients, ok := hub.clients[tournamentId]
	if !ok {
		fmt.Printf("[Broadcast] No client map found for tournament %s\n", tournamentId)
		return
	}

	if len(clients) == 0 {
		fmt.Printf("[Broadcast] Tournament %s has a client map, but it's empty\n", tournamentId)
		return
	}

	fmt.Println("Broadcasting to",len(hub.clients[tournamentId]))

	for clientId,conn := range clients {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Broadcast error to client", clientId, ":", err)
			conn.Close()
			delete(clients,clientId)
		}
	}
}

func (h *Handler) WsHandler(w http.ResponseWriter,r *http.Request){
	tournamentId := r.URL.Query().Get("tournamentId")
	clientId := r.URL.Query().Get("clientId")
	if tournamentId == "" || clientId == "" {
		http.Error(w, "Missing tournamentId/clientId",http.StatusBadRequest)
		return
	}
	fmt.Println("ws connection request detected for tournament",tournamentId,clientId)
	conn,err := upgrader.Upgrade(w,r,nil)
	if err != nil {
		fmt.Println("WebSocket upgrade failed:", err)
		http.Error(w,"Could not upgrade http conection to ws",http.StatusInternalServerError)
		return
	}
	if conn != nil {
		fmt.Println("Conn OK")
	}
	

	h.WebSocketHub.AddClient(tournamentId,clientId,conn)
	go h.handleConnection(tournamentId, clientId, conn)

}