package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type IPA interface {
	SyncHosts(ctx context.Context) error
	GetHosts(ctx context.Context) ([]db.Host, error)
	GetHostGroups(ctx context.Context) ([]string, error)
	GetHostsForGroups(ctx context.Context, groups []string) ([]db.Host, error)
}

type HostInventoryRepository interface {
	GetHostByFQDN(ctx context.Context, fqdn string) (*db.Host, error)
}

type FreeIPAService struct {
	config     conf.Config
	repository HostInventoryRepository
	log        *golog.Logger
}

// NewFreeIPAService creates a FreeIPA-backed inventory service.
func NewFreeIPAService(config conf.Config, repository HostInventoryRepository, logger *golog.Logger) (ipa *FreeIPAService) {
	ipa = &FreeIPAService{
		config:     config,
		repository: repository,
		log:        serviceLogger(logger, "[IPA]", golog.BoldPurple),
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

// GetHosts returns FreeIPA hosts visible to the operation bind user.
func (s *FreeIPAService) GetHosts(ctx context.Context) (hosts []db.Host, err error) {
	var (
		conn    *ldap.Conn
		request *ldap.SearchRequest
		result  *ldap.SearchResult
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if s.log != nil {
		s.log.Infof("Loading hosts from FreeIPA base=%s\n", s.config.FreeIPA.HostBaseDN)
	}

	if conn, err = s.connectAndBind(); err != nil {
		return
	}
	defer conn.Close()

	request = ldap.NewSearchRequest(
		s.config.FreeIPA.HostBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		"(|(objectClass=ipaHost)(fqdn=*))",
		[]string{"dn", "cn", "fqdn", "memberOf"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search hosts: %w", err)
		return
	}

	hosts = make([]db.Host, 0, len(result.Entries))
	for _, entry := range result.Entries {
		hosts = append(hosts, s.mergeStoredHost(ctx, s.ldapEntryToHost(entry, nil)))
	}

	sortHosts(hosts)
	if s.log != nil {
		s.log.Infof("Loaded %d FreeIPA hosts\n", len(hosts))
	}

	return
}

// mergeStoredHost overlays persisted Ansible inventory onto a live FreeIPA host.
func (s *FreeIPAService) mergeStoredHost(ctx context.Context, ipaHost db.Host) (host db.Host) {
	var (
		stored *db.Host
		err    error
	)

	host = ipaHost
	if s.repository == nil || host.FQDN == "" {
		return
	}

	if stored, err = s.repository.GetHostByFQDN(ctx, host.FQDN); err != nil || stored == nil {
		return
	}

	host.ID = stored.ID
	host.OSName = stored.OSName
	host.OSVersion = stored.OSVersion
	host.Arch = stored.Arch
	host.Kernel = stored.Kernel
	host.AgentVersion = stored.AgentVersion
	host.NetworkAddresses = stored.NetworkAddresses
	host.LastSeenAt = laterTime(host.LastSeenAt, stored.LastSeenAt)
	host.LastInventoryAt = stored.LastInventoryAt
	host.LastHealthAt = stored.LastHealthAt
	host.LastUpdateAt = stored.LastUpdateAt
	host.CreatedAt = stored.CreatedAt
	host.UpdatedAt = stored.UpdatedAt
	host.ProcessorModel = stored.ProcessorModel
	host.ProcessorCount = stored.ProcessorCount
	host.ProcessorCores = stored.ProcessorCores
	host.ProcessorThreads = stored.ProcessorThreads
	host.MemoryMB = stored.MemoryMB
	host.Disks = stored.Disks
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

	if s.log != nil {
		s.log.Infof("Loading host groups from FreeIPA base=%s\n", s.config.FreeIPA.HostGroupBaseDN)
	}

	if conn, err = s.connectAndBind(); err != nil {
		return
	}
	defer conn.Close()

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

	if s.log != nil {
		s.log.Infof("Loaded %d FreeIPA host groups\n", len(hostGroups))
	}

	return
}

// GetHostsForGroups expands FreeIPA host groups into concrete host targets.
func (s *FreeIPAService) GetHostsForGroups(ctx context.Context, groups []string) (hosts []db.Host, err error) {
	var (
		conn       *ldap.Conn
		seenHosts  map[string]struct{}
		seenGroups map[string]struct{}
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if s.log != nil {
		s.log.Infof("Resolving host groups into hosts groups=%v\n", normalizeStringList(groups))
	}

	if conn, err = s.connectAndBind(); err != nil {
		return
	}
	defer conn.Close()

	hosts = []db.Host{}
	seenHosts = map[string]struct{}{}
	seenGroups = map[string]struct{}{}

	for _, group := range normalizeStringList(groups) {
		if err = s.collectHostsForGroup(ctx, conn, group, seenGroups, seenHosts, &hosts); err != nil {
			return
		}
	}

	sortHosts(hosts)
	if s.log != nil {
		s.log.Infof("Resolved %d hosts from groups=%v\n", len(hosts), normalizeStringList(groups))
	}

	return
}

// collectHostsForGroup recursively collects host members for a host group.
func (s *FreeIPAService) collectHostsForGroup(ctx context.Context, conn *ldap.Conn, group string, seenGroups map[string]struct{}, seenHosts map[string]struct{}, hosts *[]db.Host) (err error) {
	var (
		groupEntry *ldap.Entry
		groupKey   string
		member     string
		members    []string
		ok         bool
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if s.log != nil {
		s.log.Debugf("Resolving host group %s\n", group)
	}

	if groupEntry, err = s.findHostGroup(ctx, conn, group); err != nil {
		return
	}

	groupKey = strings.ToLower(groupEntry.DN)
	if _, ok = seenGroups[groupKey]; ok {
		return
	}

	seenGroups[groupKey] = struct{}{}
	members = hostGroupMemberValues(groupEntry)
	if s.log != nil {
		s.log.Debugf("Host group %s direct_members=%d member_attrs=%d member_host_attrs=%d\n", groupEntry.GetAttributeValue("cn"), len(members), len(groupEntry.GetAttributeValues("member")), len(groupEntry.GetAttributeValues("memberHost")))
	}

	if err = s.collectHostsWithMemberOf(ctx, conn, groupEntry, seenHosts, hosts); err != nil {
		return
	}

	for _, member = range members {
		if dnWithinBase(member, s.config.FreeIPA.HostGroupBaseDN) {
			if err = s.collectHostsForGroup(ctx, conn, member, seenGroups, seenHosts, hosts); err != nil {
				return
			}

			continue
		}

		if looksLikeDN(member) {
			if err = s.collectHostByDN(ctx, conn, member, groupEntry.GetAttributeValue("cn"), seenHosts, hosts); err != nil {
				return
			}

			continue
		}

		addHostTarget(hostTargetFromName(member, groupEntry.GetAttributeValue("cn")), seenHosts, hosts)
	}

	return
}

// findHostGroup finds a host group by CN or DN.
func (s *FreeIPAService) findHostGroup(ctx context.Context, conn *ldap.Conn, group string) (entry *ldap.Entry, err error) {
	var (
		request *ldap.SearchRequest
		result  *ldap.SearchResult
		baseDN  string = s.config.FreeIPA.HostGroupBaseDN
		scope   int    = ldap.ScopeWholeSubtree
		filter  string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if strings.Contains(group, "=") && strings.Contains(group, ",") {
		baseDN = group
		scope = ldap.ScopeBaseObject
		filter = "(objectClass=*)"
	} else {
		filter = fmt.Sprintf(
			"(&(|(objectClass=ipaHostGroup)(objectClass=groupOfNames)(objectClass=posixGroup))(cn=%s))",
			ldap.EscapeFilter(group),
		)
	}

	request = ldap.NewSearchRequest(
		baseDN,
		scope,
		ldap.NeverDerefAliases,
		1,
		0,
		false,
		filter,
		[]string{"dn", "cn", "member", "memberHost"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search host group %q: %w", group, err)
		return
	}

	if len(result.Entries) != 1 {
		err = fmt.Errorf("host group %q not found", group)
		return
	}

	entry = result.Entries[0]
	return
}

// collectHostsWithMemberOf collects hosts that reference a host group through memberOf.
func (s *FreeIPAService) collectHostsWithMemberOf(ctx context.Context, conn *ldap.Conn, groupEntry *ldap.Entry, seenHosts map[string]struct{}, hosts *[]db.Host) (err error) {
	var (
		request *ldap.SearchRequest
		result  *ldap.SearchResult
	)

	if err = ctx.Err(); err != nil {
		return
	}

	request = ldap.NewSearchRequest(
		s.config.FreeIPA.HostBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		fmt.Sprintf("(&(fqdn=*)(memberOf=%s))", ldap.EscapeFilter(groupEntry.DN)),
		[]string{"dn", "cn", "fqdn", "memberOf"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("search hosts for group %q: %w", groupEntry.GetAttributeValue("cn"), err)
		return
	}

	for _, entry := range result.Entries {
		addHostTarget(s.ldapEntryToHost(entry, []string{groupEntry.GetAttributeValue("cn")}), seenHosts, hosts)
	}

	return
}

// collectHostByDN loads and records a single host target by LDAP DN.
func (s *FreeIPAService) collectHostByDN(ctx context.Context, conn *ldap.Conn, hostDN string, groupName string, seenHosts map[string]struct{}, hosts *[]db.Host) (err error) {
	var (
		request *ldap.SearchRequest
		result  *ldap.SearchResult
	)

	if err = ctx.Err(); err != nil {
		return
	}

	request = ldap.NewSearchRequest(
		hostDN,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1,
		0,
		false,
		"(objectClass=*)",
		[]string{"dn", "cn", "fqdn", "memberOf"},
		nil,
	)

	if result, err = conn.Search(request); err != nil {
		err = fmt.Errorf("load host %q: %w", hostDN, err)
		return
	}

	if len(result.Entries) != 1 {
		err = fmt.Errorf("host %q not found", hostDN)
		return
	}

	addHostTarget(s.ldapEntryToHost(result.Entries[0], []string{groupName}), seenHosts, hosts)
	return
}

// connectAndBind creates an LDAP connection and binds the operation user.
func (s *FreeIPAService) connectAndBind() (conn *ldap.Conn, err error) {
	if conn, err = s.connect(); err != nil {
		return
	}

	if err = conn.Bind(s.config.FreeIPA.BindDN, s.config.FreeIPA.BindPassword); err != nil {
		conn.Close()
		err = fmt.Errorf("bind operation user: %w", err)
		return
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

// ldapEntryToHost maps a FreeIPA host LDAP entry into a host model.
func (s *FreeIPAService) ldapEntryToHost(entry *ldap.Entry, hostgroups []string) (host db.Host) {
	host.FQDN = strings.TrimSpace(entry.GetAttributeValue("fqdn"))
	host.Hostname = strings.TrimSpace(entry.GetAttributeValue("cn"))
	if host.FQDN == "" {
		host.FQDN = host.Hostname
	}

	if host.Hostname == "" {
		host.Hostname = strings.Split(host.FQDN, ".")[0]
	}

	host.IPAHostDN = entry.DN
	host.Hostgroups = uniqueHostGroupNames(append(hostgroups, s.hostGroupsFromMemberOf(entry.GetAttributeValues("memberOf"))...))
	host.LastSeenAt = time.Now().UTC()
	return
}

// hostGroupsFromMemberOf extracts only FreeIPA hostgroup names from memberOf DNs.
func (s *FreeIPAService) hostGroupsFromMemberOf(memberOf []string) (hostgroups []string) {
	var (
		groupDN string
		name    string
	)

	hostgroups = []string{}
	for _, groupDN = range memberOf {
		if !dnWithinBase(groupDN, s.config.FreeIPA.HostGroupBaseDN) {
			continue
		}

		name = dnCommonName(groupDN)
		if name == "" {
			continue
		}

		hostgroups = append(hostgroups, name)
	}

	hostgroups = normalizeStringList(hostgroups)
	return
}

// uniqueHostGroupNames trims and de-duplicates hostgroup display names.
func uniqueHostGroupNames(hostgroups []string) (unique []string) {
	var (
		seen map[string]struct{} = map[string]struct{}{}
		key  string
		name string
		ok   bool
	)

	unique = []string{}
	for _, name = range hostgroups {
		name = strings.TrimSpace(name)
		key = strings.ToLower(name)
		if key == "" {
			continue
		}

		if _, ok = seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}
		unique = append(unique, name)
	}

	return
}

// addHostTarget appends a unique host target.
func addHostTarget(host db.Host, seenHosts map[string]struct{}, hosts *[]db.Host) {
	var (
		key string = strings.ToLower(firstNonEmpty(host.FQDN, host.Hostname, host.IPAHostDN))
		ok  bool
	)

	if key == "" {
		return
	}

	if _, ok = seenHosts[key]; ok {
		return
	}

	seenHosts[key] = struct{}{}
	*hosts = append(*hosts, host)
}

// dnWithinBase reports whether a DN is inside a configured LDAP base.
func dnWithinBase(dn string, base string) (ok bool) {
	dn = strings.ToLower(strings.TrimSpace(dn))
	base = strings.ToLower(strings.TrimSpace(base))
	ok = dn == base || strings.HasSuffix(dn, ","+base)
	return
}

// dnCommonName returns the first CN value from a distinguished name.
func dnCommonName(value string) (name string) {
	var (
		parsed *ldap.DN
		err    error
	)

	if parsed, err = ldap.ParseDN(strings.TrimSpace(value)); err != nil {
		return
	}

	if len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return
	}

	for _, attribute := range parsed.RDNs[0].Attributes {
		if strings.EqualFold(attribute.Type, "cn") {
			name = strings.TrimSpace(attribute.Value)
			return
		}
	}

	return
}

// sortHosts orders hosts by FQDN for stable inventories and API responses.
func sortHosts(hosts []db.Host) {
	slices.SortFunc(hosts, func(a db.Host, b db.Host) (cmp int) {
		cmp = strings.Compare(strings.ToLower(a.FQDN), strings.ToLower(b.FQDN))
		return
	})
}

// hostGroupMemberValues returns unique direct host group member values.
func hostGroupMemberValues(entry *ldap.Entry) (members []string) {
	var (
		allMembers  []string            = append(entry.GetAttributeValues("member"), entry.GetAttributeValues("memberHost")...)
		seen        map[string]struct{} = map[string]struct{}{}
		normalized  string
		member      string
		memberFound bool
	)

	for _, member = range allMembers {
		normalized = strings.ToLower(strings.TrimSpace(member))
		if normalized == "" {
			continue
		}

		if _, memberFound = seen[normalized]; memberFound {
			continue
		}

		seen[normalized] = struct{}{}
		members = append(members, strings.TrimSpace(member))
	}

	return
}

// looksLikeDN reports whether a value is an LDAP distinguished name.
func looksLikeDN(value string) (ok bool) {
	value = strings.TrimSpace(value)
	ok = strings.Contains(value, "=") && strings.Contains(value, ",")
	return
}

// hostTargetFromName creates a host target from a FreeIPA memberHost value.
func hostTargetFromName(name string, groupName string) (host db.Host) {
	host.FQDN = strings.TrimSpace(name)
	host.Hostname = strings.Split(host.FQDN, ".")[0]
	host.Hostgroups = normalizeStringList([]string{groupName})
	host.LastSeenAt = time.Now().UTC()
	return
}

// laterTime returns the later non-zero timestamp.
func laterTime(first time.Time, second time.Time) (later time.Time) {
	later = first
	if second.After(later) {
		later = second
	}

	return
}
