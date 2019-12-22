package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"time"

	consul "github.com/hashicorp/consul/api"
)

const (
	// serviceName is the name of the service registered with Consul
	serviceName = "consul-demo"

	// consulDNSPort is the port that Consul is listening to for DNS requests. 8600 is Consul's default.
	consulDNSPort = "8600"

	// consulDNSHost is the host that Consul is listening on
	consulDNSHost = "127.0.0.1"
)

type mode struct {
	client *consul.Client
}

// client is a shared Consul client
var flagVersion = flag.String("version", "", "The protocol version to use (v1 or v2). Leave blank to round robin")

func main() {
	var err error
	var ctx = signalContext(context.Background())

	// Connect to Consul
	config := consul.DefaultConfig()
	client, err := consul.NewClient(config)
	if err != nil {
		log.Fatalf("Could not connect to Consul client: %s", err)
	}

	// Seed some chaos into the world
	rand.Seed(time.Now().Unix())

	// Determine which protocol versions have been enabled
	flag.Parse()
	versions := serverVersions
	if len(*flagVersion) > 0 {
		err = fmt.Errorf("Invalid version specified: '%s'", *flagVersion)
		for _, availableVersion := range serverVersions {
			if availableVersion.version == *flagVersion {
				versions = []*serverProtocol{availableVersion}
				err = nil
			}
		}
	}

	// Run it!
	if err == nil {
		switch flag.Arg(0) {
		case "server":
			err = newServer(client).Start(ctx, versions)
		case "client":
			err = newClient(client).Start(ctx, versions)
		case "help":
			usage()
			os.Exit(1)
		default:
			err = fmt.Errorf("Invalid mode specified")
		}
	}

	if err != nil {
		log.SetFlags(0)
		log.Printf("ERROR: %s", err)
		usage()
		os.Exit(1)
	}

	log.Println("Bye!")
}

func usage() {
	flags := log.Flags()
	log.SetFlags(0)
	defer log.SetFlags(flags)

	log.Println("Usage: consul-demo client|server|help [flags]")
	flag.PrintDefaults()

}

// signalContext will cancel the given context if an interrupt is sent to the program
// It is important to handle interrupts so that the service can successfully deregister itself
// from the catalog.
func signalContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		// Block until a signal is received.
		<-c
		cancel()
	}()

	return ctx
}
