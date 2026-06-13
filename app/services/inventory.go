package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type AnsibleInventory struct {
	Path      string
	WorkDir   string
	Hostnames []string
}

type AnsibleInventoryWriter struct {
	config conf.AnsibleConfig
	log    *golog.Logger
}

// NewAnsibleInventoryWriter creates an inventory writer for job runs.
func NewAnsibleInventoryWriter(config conf.AnsibleConfig, logger *golog.Logger) (writer *AnsibleInventoryWriter) {
	writer = &AnsibleInventoryWriter{
		config: config,
		log:    serviceLogger(logger, "[INV]", golog.BoldYellow),
	}
	return
}

// WriteJobInventory writes a constrained inventory file for one job run.
func (w *AnsibleInventoryWriter) WriteJobInventory(ctx context.Context, runID int, hosts []db.Host) (inventory AnsibleInventory, err error) {
	var (
		workDir   string
		content   strings.Builder
		written   int
		hostnames []string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if runID <= 0 {
		err = NewInvalidInputError("job run id is required for inventory", nil)
		return
	}

	if len(hosts) == 0 {
		err = NewInvalidInputError("job run has no target hosts", nil)
		return
	}

	if workDir, err = w.runWorkDir(runID); err != nil {
		return
	}

	if err = os.MkdirAll(workDir, 0750); err != nil {
		err = NewExecutionError("create job inventory directory", err)
		return
	}

	if written, err = content.WriteString("[overlord_targets]\n"); err != nil || written == 0 {
		return
	}

	hostnames = make([]string, 0, len(hosts))
	for _, host := range hosts {
		var hostname string = strings.TrimSpace(firstNonEmpty(host.FQDN, host.Hostname))
		if hostname == "" {
			continue
		}

		hostnames = append(hostnames, hostname)
		if _, err = fmt.Fprintf(&content, "%s%s\n", hostname, w.inventoryHostVars()); err != nil {
			return
		}
	}

	if len(hostnames) == 0 {
		err = NewInvalidInputError("job run has no usable target hostnames", nil)
		return
	}

	inventory = AnsibleInventory{
		Path:      filepath.Join(workDir, "inventory.ini"),
		WorkDir:   workDir,
		Hostnames: hostnames,
	}

	if err = os.WriteFile(inventory.Path, []byte(content.String()), 0640); err != nil {
		err = NewExecutionError("write job inventory", err)
		return
	}

	if w.log != nil {
		w.log.Infof("Wrote Ansible inventory run_id=%d hosts=%d path=%s\n", runID, len(inventory.Hostnames), inventory.Path)
		w.log.Debugf("Inventory hosts run_id=%d hosts=%v\n", runID, inventory.Hostnames)
	}

	return
}

// inventoryHostVars returns static per-host Ansible inventory variables.
func (w *AnsibleInventoryWriter) inventoryHostVars() (hostVars string) {
	var remoteTmp string = strings.TrimSpace(w.config.RemoteTmp)

	if remoteTmp == "" {
		return
	}

	hostVars = fmt.Sprintf(" ansible_remote_tmp=%s", remoteTmp)
	return
}

// runWorkDir returns the per-run Ansible working directory.
func (w *AnsibleInventoryWriter) runWorkDir(runID int) (workDir string, err error) {
	workDir = strings.TrimSpace(w.config.WorkDir)
	if workDir == "" {
		workDir = "run/ansible"
	}

	if workDir, err = filepath.Abs(filepath.Join(workDir, fmt.Sprintf("job-run-%d", runID))); err != nil {
		err = NewExecutionError("resolve job inventory directory", err)
		return
	}

	return
}
