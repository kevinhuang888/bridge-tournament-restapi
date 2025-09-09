package api

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type WebSocketHub struct {
	clients map[string]map[*websocket.Conn]bool
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
		clients:make(map[string]map[*websocket.Conn]bool),
		expectedClientCounts:make(map[string]int),
		OnClientCountChangeMap:make(map[string]func(int)),
	}
}

func (hub *WebSocketHub) AddClient(tournamentId string, conn *websocket.Conn){
	hub.mu.Lock()
	if hub.clients[tournamentId] == nil {
		hub.clients[tournamentId] = make(map[*websocket.Conn]bool)
	}
	hub.clients[tournamentId][conn] = true
	hub.mu.Unlock()


	fmt.Println("Client added to tournament",tournamentId)
	
	if f,ok := hub.OnClientCountChangeMap[tournamentId]; ok {
		f(len(hub.clients[tournamentId]))
	}
}

func (hub * WebSocketHub) RemoveClient(tournamentId string, conn *websocket.Conn){
	hub.mu.Lock()
	
	if hub.clients[tournamentId] != nil {
		
		delete(hub.clients[tournamentId],conn)
		conn.Close()
		hub.mu.Unlock()
		fmt.Println("Client removed from tournament",tournamentId)
		if f,ok := hub.OnClientCountChangeMap[tournamentId]; ok {
			f(len(hub.clients[tournamentId]))
		}
	}
}

func (hub *WebSocketHub) Broadcast(tournamentId string, msg interface{}){
	hub.mu.Lock()
	defer hub.mu.Unlock()

	clients,ok := hub.clients[tournamentId]
	if !ok || len(clients) == 0 {
		fmt.Printf("No clients found for tournament %s\n", tournamentId)
		return
	}

	fmt.Println("Broadcasting to",len(hub.clients[tournamentId]))

	for conn := range clients {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Broadcast error",err)
			conn.Close()
			delete(clients,conn)
		}
	}
}

func (h *Handler) WsHandler(w http.ResponseWriter,r *http.Request){
	tournamentId := r.URL.Query().Get("tournamentId")
	if tournamentId == "" {
		http.Error(w, "Missing tournamentId",http.StatusBadRequest)
		return
	}
	fmt.Println("ws connection request detected for tournament",tournamentId)
	conn,err := upgrader.Upgrade(w,r,nil)
	if err != nil {
		http.Error(w,"Could not upgrade http conection to ws",http.StatusInternalServerError)
	}

	h.WebSocketHub.AddClient(tournamentId,conn)
}