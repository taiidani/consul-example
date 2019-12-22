package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	consul "github.com/hashicorp/consul/api"
)

type (
	demoClient mode

	serviceEntry struct {
		method   string
		addr     string
		port     uint
		protocol serverProtocol
	}
)

func newClient(client *consul.Client) *demoClient {
	return &demoClient{client: client}
}

func (c *demoClient) Start(ctx context.Context, enabledProtocols []*serverProtocol) (err error) {
	const secondsBetweenPing = 1
	methods := []func(string, serverProtocol) (serviceEntry, error){
		c.resolveDNS,
		c.resolveConsul,
	}

	log.Println("Client starting")
	defer log.Println("Client stopping")

	// Send a ping every 3 seconds
	ticker := time.NewTicker(time.Second * secondsBetweenPing)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Resolve the service
			var addr serviceEntry
			for _, protocol := range enabledProtocols {
				// Determine randomly which method to look up with, DNS or Consul
				method := time.Now().Unix() % int64(len(methods))
				addr, err = methods[method](serviceName, *protocol)
				if err == nil {
					break
				}
			}

			if err != nil {
				log.Println(err)
				continue
			}

			// Ping the service
			if err := c.send(addr); err != nil {
				log.Println(err)
			}
		}
	}
}

// resolveDNS resolves the given service name to a service entry using DNS.
//
// No need to use Consul libraries to look up the hostname and port
// Just use good ol' DNS!
//
// The version is used in place of "tcp" in order to request a specific tag.
func (c *demoClient) resolveDNS(name string, protocol serverProtocol) (ret serviceEntry, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// Custom dialer to let us talk with ports other than the default 53
	resolver := net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp", net.JoinHostPort(consulDNSHost, consulDNSPort))
		},
	}

	// Look up the Service DNS record (SRV) to find the port
	_, srv, err := resolver.LookupSRV(ctx, name, protocol.version, "service.consul")
	if err != nil || len(srv) == 0 {
		return ret, fmt.Errorf("Unable to lookup SRV for service %s with DNS for version %s: %w", name, protocol.version, err)
	}

	// Look up the Address DNS record (A) to find the IP
	ips, err := resolver.LookupIPAddr(ctx, fmt.Sprintf("%s.service.consul", name))
	if err != nil {
		return ret, fmt.Errorf("Unable to lookup IP for service %s with DNS for version %s: %w", name, protocol.version, err)
	}

	ret.method = "DNS-SRV"
	ret.addr = ips[0].String()
	ret.port = uint(srv[0].Port)
	ret.protocol = protocol
	return ret, nil
}

// resolveVersion resolves the given service name and version to a service entry using Consul.
//
// Using Consul directly provides some added functionality such as:
// * Being able to query unhealthy services in the catalog
// * Looking up services based on their meta data (not just tags)
// * Being an explicit call, reducing the compexity that recursing through DNS servers might cause
func (c *demoClient) resolveConsul(name string, protocol serverProtocol) (ret serviceEntry, err error) {
	// But Consul IS required to lookup the version
	addrs, _, err := c.client.Health().Service(name, protocol.version, true, nil)
	if err != nil {
		return ret, fmt.Errorf("Unable to lookup service %s with Consul for version %s: %w", name, protocol.version, err)
	} else if len(addrs) < 1 {
		return ret, fmt.Errorf("No valid Consul services published for name %s under version %s", name, protocol.version)
	}

	ret.method = "Consul"
	ret.addr = addrs[0].Service.Address
	ret.port = uint(addrs[0].Service.Port)
	ret.protocol = protocol
	return ret, nil
}

// send will make a connection to the target server and perform a round-trip request.
func (c *demoClient) send(svc serviceEntry) error {
	addr := fmt.Sprintf("%s:%d", svc.addr, svc.port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("Unable to dial a connection to %s: %w", addr, err)
	}
	defer conn.Close()

	fmt.Printf("Looked up with %-7s  Address: %-40s Sending %-4s using the %s protocol...", svc.method, addr, svc.protocol.expect, svc.protocol.version)
	_, err = fmt.Fprintln(conn, svc.protocol.expect)
	if err != nil {
		return fmt.Errorf("Unable to send data to connection: %w", err)
	}

	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("Unable to receive data back from server: %w", err)
	}

	fmt.Printf("Received %-4s in response\n", strings.TrimSpace(status))
	if strings.TrimSpace(status) != svc.protocol.reply {
		log.Println("WARNING: Message received did not match expected message" + svc.protocol.reply)
	}
	return nil
}
