//go:build darwin

package tun

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/containernetworking/cni/pkg/types"
	log "github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/tun"

	"github.com/wencaiwulue/kubevpn/pkg/config"
)

func createTun(cfg Config) (conn net.Conn, itf *net.Interface, err error) {
	ip, _, err := net.ParseCIDR(cfg.Addr)
	if err != nil {
		return
	}

	mtu := cfg.MTU
	if mtu <= 0 {
		mtu = config.DefaultMTU
	}

	var ifce tun.Device
	ifce, err = tun.CreateTUN("utun", mtu)
	if err != nil {
		return
	}

	var name string
	name, err = ifce.Name()
	if err != nil {
		return
	}

	cmd := fmt.Sprintf("ifconfig %s inet %s %s mtu %d up", name, cfg.Addr, ip.String(), mtu)
	log.Debugf("[tun] %s", cmd)
	args := strings.Split(cmd, " ")
	if er := exec.Command(args[0], args[1:]...).Run(); er != nil {
		err = fmt.Errorf("%s: %v", cmd, er)
		return
	}
	if err = os.Setenv(config.EnvTunNameOrLUID, name); err != nil {
		return nil, nil, err
	}

	if err = addTunRoutes(name, cfg.Routes...); err != nil {
		return
	}

	itf, err = net.InterfaceByName(name)
	if err != nil {
		return
	}

	conn = &tunConn{
		ifce: ifce,
		addr: &net.IPAddr{IP: ip},
	}
	return
}

func addTunRoutes(ifName string, routes ...types.Route) error {
	for _, route := range routes {
		if route.Dst.String() == "" {
			continue
		}
		cmd := fmt.Sprintf("route add -net %s -interface %s", route.Dst.String(), ifName)
		log.Debugf("[tun] %s", cmd)
		args := strings.Split(cmd, " ")
		err := exec.Command(args[0], args[1:]...).Run()
		if err != nil {
			return fmt.Errorf("%s: %v", cmd, err)
		}
	}
	return nil
}

func getInterface() (*net.Interface, error) {
	return net.InterfaceByName(os.Getenv(config.EnvTunNameOrLUID))
}
