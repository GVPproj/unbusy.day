package auth

import (
	"context"
	"errors"
	"net"
	"net/mail"
	"strings"
)

// Resolver is the DNS seam for MX validation (prior art: the Mailer/Publisher
// seams). *net.Resolver satisfies it natively; net.DefaultResolver is the
// production default.
type Resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
}

// deliverable cheaply rejects obviously-bogus addresses before sending:
// syntactic check via net/mail, then an MX lookup on the domain. It fails open
// on transient DNS errors (a resolver hiccup must not lock out a real user) and
// rejects only on definitive bad-syntax / no-MX.
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
		// Definitive "no such domain" is a reject; any other error (timeout,
		// server misbehaving) fails open so we still mail.
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return false
		}
		return true
	}
	return len(mx) > 0
}
