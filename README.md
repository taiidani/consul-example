# Consul Example

This demo consists of a server and a client that communicate over a network connection. It is intended to show some of the networking and service discovery problems that Consul addresses.

The basic behavior of the application is simple:

1. The server will start listening and register itself in Consul.
1. The client will use Consul to discover the address of the server.
1. The client will send a "ping" to the server, and the server will respond with "pong".

The demo has been laden down with intentionally complex behavior beyond this to illustrate real-world problems that some services face:

* Every few seconds the server will restart under a different random port
* Every time the server restarts, it may switch to a "v2" protocol that uses "syn/ack" instead of "v1"'s "ping/pong".
* The server itself will deregister from Consul as it restarts, causing a brief drop in service availability.
* The client will switch between DNS-based service discovery and the Consul HTTP API repeatedly between each request.

## Getting Started

First, build the application. You will need Go 1.13+ installed for this to succeed.

```
go build
```

This will generate a `consul-demo` binary in your directory. It supports one of two modes, "server" and "client", which is a required argument. You can also specify a `-version` flag if you want to lock it down to using only the "v1" or "v2" protocol.

### Set Up Your DNS

Add "127.0.0.1" to your system's DNS resolvers. This is to ensure that services on your machine reach out to Consul for discovery attempts.

### Start the Applications

Next, you're going to need **three** terminal sessions open. These will be referred to as the "consul", "server", and "client" sessions.

Run the following in each session to start Consul, a server, and a client.

| Session | Command | Purpose
| ---     | ---     | ---
| consul  | `sudo consul agent -dev -dns-port=53 -recursor=1.1.1.1` | Consul itself, providing the service catalog, DNS server, and health checks.
| server  | `./consul-demo server` | The demo server which will be listening for requests from the client.
| client  | `./consul-demo client` | The demo client which will discover the server and send requests to it.

### What Are We Seeing Here?

* "consul" should have a stream of messages logging chatter from the server and client as they discover each other over Consul.
* "server" should be emitting messages that it's receiving from the client. Every few seconds it should be shutting down and restarting on another port.
* "client" should be constantly sending network packets to the server and logging its response.

In a classic hosting environment, since the server keeps shuffling ports the client will lose track of the service and need to be manually reconfigured for the new address. But with Consul in the mix the client is able to follow the service as it republishes itself in each new location.

The server is randomly changing protocols, but the client is keeping up with it due to the server publishing its protocol version as a discoverable tag. This tells the client exactly what protocol version to use when contacting it.

For a visual, Consul should also have a UI available at http://localhost:8500. Take a look at the registered "consul-demo" service and watch as the server reregisters itself under each port. If you refresh at the right time you may also see the healch check failing which sometimes spots the server as it shuts down.

#### High Availability

You may see the occasional "no such host" error from the client if it tries to discover the server while it is shut down. This isn't very highly available! How can we solve this?

The answer is to scale horizonally. This allows a second server to pick up the lost connections while the first one is restarting. Open up another terminal session (let's call this "server2") and run the server command again:

```
./consul-demo server
```

**Gotcha:** Be careful not to start "server2" at the exact time that "server" is restarting. They should be staggered so that they can balance load properly. In a Production-like scenario your deployment tooling should take care of this blue-green style of service rollout.

You should see a second server start up in your session as well as in the Consul UI. As it starts, the client should begin discovering it as well, causing its requests to round robin (ish -- it's a demo) between the two servers. You also shouldn't see the error messages anymore!

## Troubleshooting

> I am always seeing a "no such host" for the DNS lookup

This is probably due to your system configuration. Make sure that your machine's DNS settings have "127.0.0.1" set as a resolver.
