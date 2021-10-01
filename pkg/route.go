package main

import (
	"context"
	"github.com/go-log/log"
	"github.com/pkg/errors"
	"kubevpn/core"
	"net"
	"strings"
)

type route struct {
	ServeNodes []string // tun
	ChainNodes string   // socks5
	Retries    int
}

func (r *route) parseChain() (*core.Chain, error) {
	chain := core.NewChain()
	chain.Retries = r.Retries
	gid := 1 // group ID

	ngroup := core.NewNodeGroup()
	ngroup.ID = gid
	gid++

	// parse the base nodes
	nodes, err := parseChainNode(r.ChainNodes)
	if err != nil {
		return nil, err
	}

	ngroup.AddNode(nodes...)

	chain.AddNodeGroup(ngroup)

	return chain, nil
}

func parseChainNode(ns string) (nodes []core.Node, err error) {
	node, err := core.ParseNode(ns)
	if err != nil {
		return
	}
	serverName, sport, _ := net.SplitHostPort(node.Addr)
	if serverName == "" {
		serverName = "localhost" // default server name
	}
	timeout := node.GetDuration("timeout")

	host := node.Get("host")
	if host == "" {
		host = node.Host
	}

	node.DialOptions = append(node.DialOptions,
		core.TimeoutDialOption(timeout),
		core.HostDialOption(host),
	)

	node.ConnectOptions = []core.ConnectOption{
		core.UserAgentConnectOption(node.Get("agent")),
		core.NoTLSConnectOption(node.GetBool("notls")),
		core.NoDelayConnectOption(node.GetBool("nodelay")),
	}

	handshakeOptions := []core.HandshakeOption{
		core.AddrHandshakeOption(node.Addr),
		core.HostHandshakeOption(host),
		core.IntervalHandshakeOption(node.GetDuration("ping")),
		core.TimeoutHandshakeOption(timeout),
		core.RetryHandshakeOption(node.GetInt("retry")),
	}

	node.Client = &core.Client{
		Connector:   core.AutoConnector(node.User),
		Transporter: core.TCPTransporter(),
	}

	ips := parseIP(node.Get("ip"), sport)
	for _, ip := range ips {
		nd := node.Clone()
		nd.Addr = ip
		// override the default node address
		nd.HandshakeOptions = append(handshakeOptions, core.AddrHandshakeOption(ip))
		// One node per IP
		nodes = append(nodes, nd)
	}
	if len(ips) == 0 {
		node.HandshakeOptions = handshakeOptions
		nodes = []core.Node{node}
	}

	return
}

func (r *route) GenRouters() ([]router, error) {
	chain, err := r.parseChain()
	if err != nil {
		if !errors.Is(err, core.ErrInvalidNode) {
			return nil, err
		}
	}

	routers := make([]router, 0, len(r.ServeNodes))
	for _, serveNode := range r.ServeNodes {
		node, err := core.ParseNode(serveNode)
		if err != nil {
			return nil, err
		}
		ttl := node.GetDuration("ttl")
		timeout := node.GetDuration("timeout")

		tunRoutes := parseIPRoutes(node.Get("route"))
		gw := net.ParseIP(node.Get("gw")) // default gateway
		for i := range tunRoutes {
			if tunRoutes[i].Gateway == nil {
				tunRoutes[i].Gateway = gw
			}
		}

		var ln core.Listener
		switch node.Transport {
		case "tcp":
			ln, err = core.TCPListener(node.Addr)
		case "udp":
			ln, err = core.UDPListener(node.Addr, &core.UDPListenConfig{
				TTL:       ttl,
				Backlog:   node.GetInt("backlog"),
				QueueSize: node.GetInt("queue"),
			})
		case "tun":
			cfg := core.TunConfig{
				Name:    node.Get("name"),
				Addr:    node.Get("net"),
				Peer:    node.Get("peer"),
				MTU:     node.GetInt("mtu"),
				Routes:  tunRoutes,
				Gateway: node.Get("gw"),
			}
			ln, err = core.TunListener(cfg)
		case "tap":
			cfg := core.TapConfig{
				Name:    node.Get("name"),
				Addr:    node.Get("net"),
				MTU:     node.GetInt("mtu"),
				Routes:  strings.Split(node.Get("route"), ","),
				Gateway: node.Get("gw"),
			}
			ln, err = core.TapListener(cfg)
		default:
			ln, err = core.TCPListener(node.Addr)
		}
		if err != nil {
			return nil, err
		}

		var handler core.Handler
		switch node.Protocol {
		case "tun":
			handler = core.TunHandler()
		case "tap":
			handler = core.TapHandler()
		default:
			handler = core.AutoHandler()
		}

		handler.Init(
			core.AddrHandlerOption(ln.Addr().String()),
			core.ChainHandlerOption(chain),
			core.AuthenticatorHandlerOption(core.DefaultAuthenticator),
			core.RetryHandlerOption(node.GetInt("retry")),
			core.TimeoutHandlerOption(timeout),
			core.NodeHandlerOption(node),
			core.TCPModeHandlerOption(node.GetBool("tcp")),
			core.IPRoutesHandlerOption(tunRoutes...),
		)

		rt := router{
			node:    node,
			server:  &core.Server{Listener: ln},
			handler: handler,
			chain:   chain,
		}
		routers = append(routers, rt)
	}

	return routers, nil
}

type router struct {
	node    core.Node
	server  *core.Server
	handler core.Handler
	chain   *core.Chain
}

func (r *router) Serve(ctx context.Context) error {
	log.Logf("%s on %s", r.node.String(), r.server.Addr())
	return r.server.Serve(ctx, r.handler)
}

func (r *router) Close() error {
	if r == nil || r.server == nil {
		return nil
	}
	return r.server.Close()
}
