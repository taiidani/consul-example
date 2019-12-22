package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"time"

	consul "github.com/hashicorp/consul/api"
)

type (
	demoServer mode

	serverProtocol struct {
		version string // The version of the protocol
		send    string // The message to expect from a client
		reply   string // The reply to send to the client
	}
)

// serverVersions is a cheap way to publish a shared protocol
// This is equivalent to, say, a Swagger document.
var serverVersions = []*serverProtocol{
	{
		version: "v2",
		send:    "syn",
		reply:   "ack",
	},
	{
		version: "v1",
		send:    "ping",
		reply:   "pong",
	},
}

func newServer(client *consul.Client) *demoServer {
	return &demoServer{client: client}
}

func (s *demoServer) Start(ctx context.Context, enabledProtocols []*serverProtocol) error {
	const secondsBetweenRefresh = 10

	log.Println("Starting server")
	defer log.Println("Server shut down")

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			ctx, cancel := context.WithTimeout(ctx, time.Second*secondsBetweenRefresh)
			defer cancel()

			// Assign server to a random port
			port := rand.Intn(10000) + 10000
			version := rand.Intn(len(enabledProtocols))

			err := s.begin(ctx, port, enabledProtocols[version])
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func (s *demoServer) begin(ctx context.Context, port int, protocol *serverProtocol) error {
	// Register service
	serviceID, err := s.register(serviceName, port, protocol)
	if err != nil {
		return err
	}
	defer s.client.Agent().ServiceDeregister(serviceID)

	// Set up the network connection
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("Unable to bind to address: %w", err)
	}
	log.Printf("Server listening on %s using the %s protocol\n", addr, protocol.version)

	// Start accepting connections
	s.loop(ctx, listener, protocol)
	return nil
}

// register registers the service address and information in Consul
// Most real-world services will not need to do this as the service registration
// logic will be handled outside of the codebase (e.g. container orchestrator)
func (s *demoServer) register(name string, port int, protocol *serverProtocol) (string, error) {
	serviceID := fmt.Sprintf("%s:%d", name, port)

	reg := &consul.AgentServiceRegistration{
		ID:   serviceID,
		Name: name,
		Port: port,
		Check: &consul.AgentServiceCheck{
			CheckID:  "health-" + serviceID,
			Interval: "1s",
			TCP:      fmt.Sprintf("127.0.0.1:%d", port),
		},
		Tags: []string{protocol.version},
		Meta: map[string]string{
			"version": protocol.version,
		},
	}
	err := s.client.Agent().ServiceRegister(reg)
	if err != nil {
		err = fmt.Errorf("Unable to register service in Consul: %w", err)
	}

	return serviceID, err
}

func (s *demoServer) loop(ctx context.Context, listener net.Listener, protocol *serverProtocol) {
	go func() {
		// Shutdown server upon cancellation
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Shutting down listener")
			break
		}

		go func(conn net.Conn) {
			defer conn.Close()

			body, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil && err == io.EOF {
				// Telnet and healthchecks don't send any data
				return
			} else if err != nil {
				log.Printf("❌ ERROR: Unable to read data from connection: %s", err)
				return
			}

			if strings.TrimSpace(body) != protocol.send {
				log.Printf("❌ WARNING: Message received '%s'did not match expected message %s", strings.TrimSpace(body), protocol.send)
			}

			// Write back
			fmt.Printf("✔️ Received data '%s'. Returning data '%s'.\n", strings.TrimSpace(body), protocol.reply)
			_, err = fmt.Fprintln(conn, protocol.reply)
			if err != nil {
				log.Printf("❌ ERROR: Unable to send data to incoming connection: %s", err)
			}
		}(conn)
	}
}
