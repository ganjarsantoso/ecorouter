package server

import (
	"net"
	"strings"

	"github.com/ganjar/ecorouter/internal/config"
)

// AccessControl enforces optional allow/deny CIDR lists.
type AccessControl struct {
	allow []*net.IPNet
	deny  []*net.IPNet
}

func NewAccess(cfg config.AccessConfig) (*AccessControl, error) {
	ac := &AccessControl{}
	for _, c := range cfg.Allow {
		n, err := parseCIDR(c)
		if err != nil {
			return nil, err
		}
		ac.allow = append(ac.allow, n)
	}
	for _, c := range cfg.Deny {
		n, err := parseCIDR(c)
		if err != nil {
			return nil, err
		}
		ac.deny = append(ac.deny, n)
	}
	return ac, nil
}

func parseCIDR(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		// single IP
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, errString("invalid IP/CIDR: " + s)
		}
		if ip.To4() != nil {
			s = s + "/32"
		} else {
			s = s + "/128"
		}
	}
	_, n, err := net.ParseCIDR(s)
	return n, err
}

// Allowed returns false if the IP is denied.
// Empty allow list = open (anywhere). Deny always applies.
func (ac *AccessControl) Allowed(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, d := range ac.deny {
		if d.Contains(ip) {
			return false
		}
	}
	if len(ac.allow) == 0 {
		return true
	}
	for _, a := range ac.allow {
		if a.Contains(ip) {
			return true
		}
	}
	return false
}
