package auth

import (
	"context"
	"errors"
	"net"
	"net/mail"
	"strings"
)

// Resolver is the DNS seam for MX validation; *net.Resolver satisfies it.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

// deliverable rejects only definitive bad-syntax / no-MX addresses; it fails
// open on transient DNS errors so a resolver hiccup can't lock out a real user.
func (s *Service) deliverable(ctx context.Context, email string) bool {
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	at := strings.LastIndex(addr.Address, "@")
	if at < 0 {
		return false
	}
	domain := addr.Address[at+1:]

	mx, err := s.resolver.LookupMX(ctx, domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false
		}
		return true
	}
	return len(mx) > 0
}
