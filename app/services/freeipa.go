package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type IPA interface {
	SyncHosts(ctx context.Context) error
	GetHosts(ctx context.Context) ([]db.Host, error)
	GetHostGroups(ctx context.Context) ([]string, error)
}

type FreeIPAService struct {
	config conf.Config
}

// NewFreeIPAService creates a FreeIPA-backed inventory service.
func NewFreeIPAService(config conf.Config) (ipa *FreeIPAService) {
	ipa = &FreeIPAService{
		config: config,
	}
	return
}

// SyncHosts validates context cancellation until inventory sync is implemented.
func (s *FreeIPAService) SyncHosts(ctx context.Context) (err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	return
}

// GetHosts returns no local filler inventory before real sync exists.
func (s *FreeIPAService) GetHosts(ctx context.Context) (hosts []db.Host, err error) {
	if err = ctx.Err(); err != nil {
		return
	}

	hosts = []db.Host{}
	return
}

// GetHostGroups returns FreeIPA host group names using the operation bind user.
func (s *FreeIPAService) GetHostGroups(ctx context.Context) (hostGroups []string, err error) {
	var (
		conn    *ldap.Conn
		request *ldap.SearchRequest
		result  *ldap.SearchResult
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if conn, err = s.connect(); err != nil {
		return
	}
	defer conn.Close()

	if err = conn.Bind(s.config.FreeIPA.BindDN, s.config.FreeIPA.BindPassword); err != nil {
		err = fmt.Errorf("bind operation user: %w", err)
		return
	}

	request = ldap.NewSearchRequest(
		s.config.FreeIPA.HostGroupBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(|(objectClass=ipaHostGroup)(objectClass=groupOfNames)(objectClass=posixGroup))",
		[]string{"cn"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search host groups: %w", err)
		return
	}

	hostGroups = make([]string, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if entry.GetAttributeValue("cn") != "" {
			hostGroups = append(hostGroups, entry.GetAttributeValue("cn"))
		}
	}

	return
}

// connect creates a configured LDAP connection for FreeIPA inventory queries.
func (s *FreeIPAService) connect() (conn *ldap.Conn, err error) {
	var (
		tlsConfig      *tls.Config
		connectTimeout time.Duration
		requestTimeout time.Duration
		scheme         string = "ldap"
		addr           string
	)

	if tlsConfig, err = s.tlsConfig(); err != nil {
		return
	}

	if connectTimeout, err = time.ParseDuration(s.config.FreeIPA.ConnectTimeout); err != nil {
		err = fmt.Errorf("parse ldap connect timeout: %w", err)
		return
	}

	if requestTimeout, err = time.ParseDuration(s.config.FreeIPA.RequestTimeout); err != nil {
		err = fmt.Errorf("parse ldap request timeout: %w", err)
		return
	}

	if s.config.FreeIPA.UseTLS && !s.config.FreeIPA.StartTLS {
		scheme = "ldaps"
	}

	addr = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(s.config.FreeIPA.Server, fmt.Sprint(s.config.FreeIPA.Port)))
	if conn, err = ldap.DialURL(
		addr,
		ldap.DialWithDialer(&net.Dialer{Timeout: connectTimeout}),
		ldap.DialWithTLSConfig(tlsConfig),
	); err != nil {
		err = fmt.Errorf("connect ldap: %w", err)
		return
	}

	conn.SetTimeout(requestTimeout)

	if s.config.FreeIPA.StartTLS {
		if err = conn.StartTLS(tlsConfig); err != nil {
			conn.Close()
			err = fmt.Errorf("start tls: %w", err)
			return
		}
	}

	return
}

// tlsConfig builds TLS settings for FreeIPA LDAP connections.
func (s *FreeIPAService) tlsConfig() (config *tls.Config, err error) {
	var (
		certPEM []byte
		pool    *x509.CertPool
		ok      bool
	)

	config = &tls.Config{
		ServerName:         s.config.FreeIPA.Server,
		InsecureSkipVerify: s.config.FreeIPA.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}

	if s.config.FreeIPA.CACertFile == "" {
		return
	}

	if certPEM, err = os.ReadFile(s.config.FreeIPA.CACertFile); err != nil {
		err = fmt.Errorf("read ldap ca cert: %w", err)
		return
	}

	if pool, err = x509.SystemCertPool(); err != nil {
		pool = x509.NewCertPool()
	}

	if ok = pool.AppendCertsFromPEM(certPEM); !ok {
		err = fmt.Errorf("parse ldap ca cert")
		return
	}

	config.RootCAs = pool
	return
}
