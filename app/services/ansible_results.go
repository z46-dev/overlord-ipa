package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/z46-dev/overlord-ipa/db"
)

type ansibleHostRecap struct {
	Hostname    string
	OK          int
	Changed     int
	Unreachable int
	Failed      int
	Skipped     int
}

var ansibleRecapLinePattern *regexp.Regexp = regexp.MustCompile(`^(\S+)\s+:\s+ok=(\d+)\s+changed=(\d+)\s+unreachable=(\d+)\s+failed=(\d+)\s+skipped=(\d+)`)

// persistActionHostResults stores per-host run details and host inventory facts.
func (s *JobService) persistActionHostResults(ctx context.Context, run *db.JobRun, action db.JobAction, output AnsibleRunOutput, inventory AnsibleInventory) (err error) {
	var (
		recaps map[string]ansibleHostRecap
		facts  map[string]map[string]any
		now    time.Time = time.Now().UTC()
	)

	if recaps, err = parseAnsibleRecaps(output.Stdout); err != nil {
		return
	}

	if facts, err = loadAnsibleFactCache(output.FactCacheDir); err != nil {
		return
	}

	if err = s.persistRecapHostResults(ctx, run, action, recaps, facts, now); err != nil {
		return
	}

	if err = s.persistFactHosts(ctx, action, facts, now); err != nil {
		return
	}

	if len(recaps) == 0 && len(facts) == 0 && len(inventory.Hostnames) > 0 {
		if s.log != nil {
			s.log.Debugf("No structured Ansible host results found run_id=%d action_id=%d\n", run.ID, action.ID)
		}
	}

	return
}

// persistRecapHostResults inserts JobHostResult rows from Ansible recap output.
func (s *JobService) persistRecapHostResults(ctx context.Context, run *db.JobRun, action db.JobAction, recaps map[string]ansibleHostRecap, facts map[string]map[string]any, now time.Time) (err error) {
	var (
		recap     ansibleHostRecap
		result    db.JobHostResult
		resultRaw []byte
		hostFacts map[string]any
		ok        bool
	)

	for _, recap = range recaps {
		hostFacts, ok = facts[strings.ToLower(recap.Hostname)]
		if ok {
			if resultRaw, err = json.Marshal(hostFacts); err != nil {
				return
			}
		} else {
			resultRaw = nil
		}

		result = db.JobHostResult{
			JobRunID:    uint64(run.ID),
			Hostname:    recap.Hostname,
			Status:      statusFromRecap(recap),
			Changed:     recap.Changed > 0,
			Unreachable: recap.Unreachable > 0,
			Message:     messageFromRecap(recap),
			ResultJSON:  string(resultRaw),
		}

		if err = s.repository.InsertJobHostResult(ctx, &result); err != nil {
			err = NewPersistenceError("insert job host result", err)
			return
		}

		if err = s.persistRecapHost(ctx, action, recap, now); err != nil {
			return
		}
	}

	return
}

// persistRecapHost updates basic host timestamps from per-host job outcomes.
func (s *JobService) persistRecapHost(ctx context.Context, action db.JobAction, recap ansibleHostRecap, now time.Time) (err error) {
	var host db.Host

	if recap.Hostname == "" || recap.Unreachable > 0 || recap.Failed > 0 {
		return
	}

	host = db.Host{
		FQDN:       recap.Hostname,
		Hostname:   strings.Split(recap.Hostname, ".")[0],
		LastSeenAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if strings.Contains(strings.ToLower(action.FilePath), "health") || strings.Contains(strings.ToLower(action.FilePath), "heartbeat") {
		host.LastHealthAt = now
	}

	if strings.Contains(strings.ToLower(action.FilePath), "inventory") {
		host.LastInventoryAt = now
	}

	if err = s.repository.UpsertHost(ctx, &host); err != nil {
		err = NewPersistenceError("upsert host recap", err)
		return
	}

	return
}

// persistFactHosts upserts Host rows from cached Ansible facts.
func (s *JobService) persistFactHosts(ctx context.Context, action db.JobAction, facts map[string]map[string]any, now time.Time) (err error) {
	var host db.Host

	for fallback, values := range facts {
		host = hostFromAnsibleFacts(values, fallback, now)
		if strings.Contains(strings.ToLower(action.FilePath), "inventory") {
			host.LastInventoryAt = now
		}

		if strings.Contains(strings.ToLower(action.FilePath), "health") || strings.Contains(strings.ToLower(action.FilePath), "heartbeat") {
			host.LastHealthAt = now
		}

		if err = s.repository.UpsertHost(ctx, &host); err != nil {
			err = NewPersistenceError("upsert host inventory", err)
			return
		}
	}

	return
}

// parseAnsibleRecaps extracts per-host play recap counters from stdout.
func parseAnsibleRecaps(stdout string) (recaps map[string]ansibleHostRecap, err error) {
	var (
		lines   []string = strings.Split(stdout, "\n")
		matches []string
		recap   ansibleHostRecap
	)

	recaps = map[string]ansibleHostRecap{}
	for _, line := range lines {
		matches = ansibleRecapLinePattern.FindStringSubmatch(line)
		if len(matches) != 7 {
			continue
		}

		recap = ansibleHostRecap{Hostname: matches[1]}
		if recap.OK, err = strconv.Atoi(matches[2]); err != nil {
			return
		}

		if recap.Changed, err = strconv.Atoi(matches[3]); err != nil {
			return
		}

		if recap.Unreachable, err = strconv.Atoi(matches[4]); err != nil {
			return
		}

		if recap.Failed, err = strconv.Atoi(matches[5]); err != nil {
			return
		}

		if recap.Skipped, err = strconv.Atoi(matches[6]); err != nil {
			return
		}

		recaps[strings.ToLower(recap.Hostname)] = recap
	}

	return
}

// loadAnsibleFactCache reads jsonfile fact cache entries from a run directory.
func loadAnsibleFactCache(factsDir string) (facts map[string]map[string]any, err error) {
	var entries []os.DirEntry

	facts = map[string]map[string]any{}
	if strings.TrimSpace(factsDir) == "" {
		return
	}

	if entries, err = os.ReadDir(factsDir); err != nil {
		if os.IsNotExist(err) {
			err = nil
		}

		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if err = loadAnsibleFactFile(filepath.Join(factsDir, entry.Name()), facts); err != nil {
			return
		}
	}

	return
}

// loadAnsibleFactFile decodes one jsonfile fact cache entry.
func loadAnsibleFactFile(path string, facts map[string]map[string]any) (err error) {
	var (
		data       []byte
		outer      map[string]json.RawMessage
		payload    string
		values     map[string]any
		hostname   string
		payloadRaw json.RawMessage
		ok         bool
	)

	if data, err = os.ReadFile(path); err != nil {
		return
	}

	if err = json.Unmarshal(data, &outer); err != nil {
		return
	}

	if payloadRaw, ok = outer["__payload__"]; ok {
		if err = json.Unmarshal(payloadRaw, &payload); err != nil {
			return
		}

		if err = json.Unmarshal([]byte(payload), &values); err != nil {
			return
		}
	} else if err = json.Unmarshal(data, &values); err != nil {
		return
	}

	hostname = strings.ToLower(firstNonEmpty(stringValue(values, "ansible_fqdn"), stringValue(values, "ansible_nodename"), strings.TrimPrefix(filepath.Base(path), "s1_")))
	if hostname == "" {
		return
	}

	facts[hostname] = values
	return
}

// hostFromAnsibleFacts maps gathered Ansible facts onto the Host model.
func hostFromAnsibleFacts(facts map[string]any, fallback string, now time.Time) (host db.Host) {
	host.FQDN = firstNonEmpty(stringValue(facts, "ansible_fqdn"), stringValue(facts, "ansible_nodename"), fallback)
	host.Hostname = firstNonEmpty(stringValue(facts, "ansible_hostname"), strings.Split(host.FQDN, ".")[0])
	host.OSName = stringValue(facts, "ansible_distribution")
	host.OSVersion = stringValue(facts, "ansible_distribution_version")
	host.Arch = stringValue(facts, "ansible_architecture")
	host.Kernel = stringValue(facts, "ansible_kernel")
	host.ProcessorModel = processorModel(facts)
	host.ProcessorCount = intValue(facts, "ansible_processor_count")
	host.ProcessorCores = intValue(facts, "ansible_processor_cores")
	host.ProcessorThreads = intValue(facts, "ansible_processor_vcpus")
	host.MemoryMB = intValue(facts, "ansible_memtotal_mb")
	host.NetworkAddresses = networkAddresses(facts)
	host.Disks = disksFromMounts(facts)
	host.LastSeenAt = now
	host.CreatedAt = now
	host.UpdatedAt = now
	return
}

// statusFromRecap maps Ansible recap counters to a host result status.
func statusFromRecap(recap ansibleHostRecap) (status db.JobHostResultStatus) {
	status = db.JobHostResultStatusSuccess
	if recap.Unreachable > 0 {
		status = db.JobHostResultStatusUnreachable
		return
	}

	if recap.Failed > 0 {
		status = db.JobHostResultStatusFailed
		return
	}

	if recap.Changed > 0 {
		status = db.JobHostResultStatusChanged
		return
	}

	if recap.OK == 0 && recap.Skipped > 0 {
		status = db.JobHostResultStatusSkipped
	}

	return
}

// messageFromRecap creates a compact host result summary.
func messageFromRecap(recap ansibleHostRecap) (message string) {
	message = fmt.Sprintf("ok=%d changed=%d unreachable=%d failed=%d skipped=%d", recap.OK, recap.Changed, recap.Unreachable, recap.Failed, recap.Skipped)
	return
}

// stringValue reads a string fact.
func stringValue(values map[string]any, key string) (value string) {
	var (
		raw any
		ok  bool
	)

	if raw, ok = values[key]; ok {
		value = fmt.Sprint(raw)
	}

	return
}

// intValue reads a numeric fact.
func intValue(values map[string]any, key string) (value int) {
	var (
		raw     any
		ok      bool
		asFloat float64
		asInt   int
		asText  string
	)

	if raw, ok = values[key]; !ok {
		return
	}

	if asFloat, ok = raw.(float64); ok {
		value = int(asFloat)
		return
	}

	if asInt, ok = raw.(int); ok {
		value = asInt
		return
	}

	if asText, ok = raw.(string); ok {
		value, _ = strconv.Atoi(asText)
	}

	return
}

// processorModel returns a useful CPU model string from Ansible facts.
func processorModel(values map[string]any) (model string) {
	var items []any

	if items, _ = values["ansible_processor"].([]any); len(items) == 0 {
		return
	}

	for _, item := range items {
		model = strings.TrimSpace(fmt.Sprint(item))
		if model != "" && !isIntegerString(model) && !strings.EqualFold(model, "GenuineIntel") && !strings.EqualFold(model, "AuthenticAMD") {
			return
		}
	}

	model = ""
	return
}

// networkAddresses extracts interface IP and MAC addresses.
func networkAddresses(values map[string]any) (addresses []db.NetworkAddressInfo) {
	var (
		interfaces []any
		ifaceName  string
		ifaceFacts map[string]any
		macAddress string
		ipv4       map[string]any
		ipv6Values []any
		ipv6       map[string]any
		ipValues   []any
		seen       map[string]struct{} = map[string]struct{}{}
	)

	addresses = []db.NetworkAddressInfo{}
	if interfaces, _ = values["ansible_interfaces"].([]any); len(interfaces) > 0 {
		for _, iface := range interfaces {
			ifaceName = fmt.Sprint(iface)
			if ifaceName == "" || ifaceName == "lo" {
				continue
			}

			if ifaceFacts, _ = values["ansible_"+ifaceName].(map[string]any); len(ifaceFacts) == 0 {
				continue
			}

			macAddress = stringValue(ifaceFacts, "macaddress")

			ipv4, _ = ifaceFacts["ipv4"].(map[string]any)
			appendNetworkAddress(&addresses, seen, stringValue(ipv4, "address"), macAddress)

			ipv6Values, _ = ifaceFacts["ipv6"].([]any)
			for _, value := range ipv6Values {
				if ipv6, _ = value.(map[string]any); len(ipv6) == 0 {
					continue
				}

				appendNetworkAddress(&addresses, seen, stringValue(ipv6, "address"), macAddress)
			}
		}
	}

	if len(addresses) > 0 {
		return
	}

	if ipValues, _ = values["ansible_all_ipv4_addresses"].([]any); len(ipValues) > 0 {
		for _, ip := range ipValues {
			appendNetworkAddress(&addresses, seen, fmt.Sprint(ip), "")
		}
	}

	if ipValues, _ = values["ansible_all_ipv6_addresses"].([]any); len(ipValues) > 0 {
		for _, ip := range ipValues {
			appendNetworkAddress(&addresses, seen, fmt.Sprint(ip), "")
		}
	}

	return
}

// appendNetworkAddress adds one non-local IP address to the inventory list.
func appendNetworkAddress(addresses *[]db.NetworkAddressInfo, seen map[string]struct{}, ipAddress string, macAddress string) {
	var (
		key string
		ok  bool
	)

	ipAddress = strings.TrimSpace(ipAddress)
	macAddress = strings.TrimSpace(macAddress)
	if !isNonLocalIPAddress(ipAddress) {
		return
	}

	key = strings.ToLower(ipAddress + "|" + macAddress)
	if _, ok = seen[key]; ok {
		return
	}

	seen[key] = struct{}{}
	*addresses = append(*addresses, db.NetworkAddressInfo{
		IPAddress:  ipAddress,
		MACAddress: macAddress,
	})
}

// isNonLocalIPAddress reports whether an IP is useful host inventory data.
func isNonLocalIPAddress(value string) (ok bool) {
	var (
		addr    netip.Addr
		err     error
		percent int
	)

	value = strings.TrimSpace(value)
	percent = strings.Index(value, "%")
	if percent >= 0 {
		value = value[:percent]
	}

	if addr, err = netip.ParseAddr(value); err != nil {
		return
	}

	ok = !addr.IsLoopback() &&
		!addr.IsUnspecified() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast()
	return
}

// disksFromMounts maps Ansible mount facts into disk summaries.
func disksFromMounts(values map[string]any) (disks []db.DiskInfo) {
	var mounts []any

	disks = []db.DiskInfo{}
	if mounts, _ = values["ansible_mounts"].([]any); len(mounts) == 0 {
		return
	}

	for _, item := range mounts {
		var mount map[string]any
		var ok bool
		var total int64
		var available int64

		if mount, ok = item.(map[string]any); !ok {
			continue
		}

		total = int64FromAny(mount["size_total"])
		available = int64FromAny(mount["size_available"])
		if total <= 0 {
			continue
		}

		disks = append(disks, db.DiskInfo{
			Name:      fmt.Sprint(mount["mount"]),
			Size:      total,
			Used:      total - available,
			Available: available,
		})
	}

	return
}

// int64FromAny converts JSON numeric values into int64.
func int64FromAny(value any) (converted int64) {
	var (
		ok      bool
		asFloat float64
		asInt64 int64
		asInt   int
		asText  string
	)

	if asFloat, ok = value.(float64); ok {
		converted = int64(asFloat)
		return
	}

	if asInt64, ok = value.(int64); ok {
		converted = asInt64
		return
	}

	if asInt, ok = value.(int); ok {
		converted = int64(asInt)
		return
	}

	if asText, ok = value.(string); ok {
		converted, _ = strconv.ParseInt(asText, 10, 64)
	}

	return
}

// isIntegerString reports whether a string is an integer.
func isIntegerString(value string) (ok bool) {
	var err error

	if value == "" {
		return
	}

	if _, err = strconv.Atoi(value); err == nil {
		ok = true
	}

	return
}
