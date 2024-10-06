package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
)

type Server struct {
	clients map[net.Conn]string
	mu      sync.Mutex
}

func NewServer() *Server {
	return &Server{
		clients: make(map[net.Conn]string),
	}
}

func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	clientID := fmt.Sprintf("player%d", len(s.clients)+1)
	s.mu.Lock()
	s.clients[conn] = clientID
	s.mu.Unlock()

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading from client:", err)
			s.mu.Lock()
			delete(s.clients, conn)
			s.mu.Unlock()
			return
		}

		s.broadcast(fmt.Sprintf("%s,%s", clientID, message))
	}
}

func (s *Server) broadcast(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for conn := range s.clients {
		_, err := fmt.Fprint(conn, message)
		if err != nil {
			log.Println("Error sending to client:", err)
		}
	}
}

func main() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("Error starting server:", err)
	}
	defer listener.Close()

	server := NewServer()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		go server.handleClient(conn)
	}
}
