package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/apenella/go-ansible/v2/pkg/adhoc"
	"github.com/apenella/go-ansible/v2/pkg/execute"
	"github.com/apenella/go-ansible/v2/pkg/playbook"
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type ActionPathResolver interface {
	ResolveActionPath(ctx context.Context, path string, actionType db.JobActionType) (string, error)
}

type AnsibleRunRequest struct {
	Action           db.JobAction
	InventoryPath    string
	WorkingDirectory string
	Limit            string
}

type AnsibleRunOutput struct {
	Stdout       string
	Stderr       string
	FactCacheDir string
}

type GoAnsibleRunner struct {
	config conf.AnsibleConfig
	files  ActionPathResolver
	log    *golog.Logger
}

// NewGoAnsibleRunner creates an Ansible command runner.
func NewGoAnsibleRunner(config conf.AnsibleConfig, files ActionPathResolver, logger *golog.Logger) (runner *GoAnsibleRunner) {
	runner = &GoAnsibleRunner{
		config: config,
		files:  files,
		log:    serviceLogger(logger, "[ANSIBLE]", golog.BoldGreen),
	}
	return
}

// RunPlaybook executes a validated data-directory playbook with an explicit inventory.
func (r *GoAnsibleRunner) RunPlaybook(ctx context.Context, request AnsibleRunRequest) (output AnsibleRunOutput, err error) {
	var (
		actionPath string
		workDir    string
		options    *playbook.AnsiblePlaybookOptions
		cmd        *playbook.AnsiblePlaybookCmd
		executor   *execute.DefaultExecute
		details    string
		factsDir   string
		stdout     bytes.Buffer
		stderr     bytes.Buffer
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if err = validateAnsibleRequest(request); err != nil {
		return
	}

	if actionPath, err = r.files.ResolveActionPath(ctx, request.Action.FilePath, db.JobActionTypeAnsiblePlaybook); err != nil {
		return
	}

	if workDir, err = r.prepareWorkDir(request.WorkingDirectory); err != nil {
		return
	}

	if factsDir, err = r.prepareFactsDir(workDir); err != nil {
		return
	}

	options = r.playbookOptions(request)
	cmd = playbook.NewAnsiblePlaybookCmd(
		playbook.WithBinary(r.playbookBinary()),
		playbook.WithPlaybookOptions(options),
		playbook.WithPlaybooks(actionPath),
	)
	executor = execute.NewDefaultExecute(
		execute.WithCmd(cmd),
		execute.WithCmdRunDir(workDir),
		execute.WithWrite(&stdout),
		execute.WithWriteError(&stderr),
		execute.WithEnvVars(r.ansibleEnv(factsDir)),
	)

	if r.log != nil {
		r.log.Infof("Running playbook action_id=%d file=%s inventory=%s work_dir=%s\n", request.Action.ID, request.Action.FilePath, request.InventoryPath, workDir)
		r.log.Debugf("Ansible command: %s\n", cmd.String())
	}

	err = executor.Execute(ctx)
	output = AnsibleRunOutput{
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		FactCacheDir: factsDir,
	}

	if err != nil {
		if r.log != nil {
			r.log.Errorf(
				"Playbook failed action_id=%d file=%s error=%v stdout=%s stderr=%s\n",
				request.Action.ID,
				request.Action.FilePath,
				err,
				truncateLog(stdout.String(), 1200),
				truncateLog(stderr.String(), 1200),
			)
		}

		details = combinedCommandOutput(output)
		if details != "" {
			err = fmt.Errorf("%w: %s", err, details)
		}

		err = NewExecutionError("run ansible playbook", err)
		return
	}

	if r.log != nil {
		r.log.Infof("Playbook completed action_id=%d stdout_bytes=%d stderr_bytes=%d\n", request.Action.ID, len(output.Stdout), len(output.Stderr))
	}

	return
}

// RunShellScript executes a validated data-directory shell script through Ansible's script module.
func (r *GoAnsibleRunner) RunShellScript(ctx context.Context, request AnsibleRunRequest) (output AnsibleRunOutput, err error) {
	var (
		actionPath string
		workDir    string
		options    *adhoc.AnsibleAdhocOptions
		cmd        *adhoc.AnsibleAdhocCmd
		executor   *execute.DefaultExecute
		details    string
		factsDir   string
		stdout     bytes.Buffer
		stderr     bytes.Buffer
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if err = validateAnsibleRequest(request); err != nil {
		return
	}

	if actionPath, err = r.files.ResolveActionPath(ctx, request.Action.FilePath, db.JobActionTypeShell); err != nil {
		return
	}

	if workDir, err = r.prepareWorkDir(request.WorkingDirectory); err != nil {
		return
	}

	if factsDir, err = r.prepareFactsDir(workDir); err != nil {
		return
	}

	options = r.adhocOptions(request, actionPath)
	cmd = adhoc.NewAnsibleAdhocCmd(
		adhoc.WithBinary(r.adhocBinary()),
		adhoc.WithPattern("all"),
		adhoc.WithAdhocOptions(options),
	)
	executor = execute.NewDefaultExecute(
		execute.WithCmd(cmd),
		execute.WithCmdRunDir(workDir),
		execute.WithWrite(&stdout),
		execute.WithWriteError(&stderr),
		execute.WithEnvVars(r.ansibleEnv(factsDir)),
	)

	if r.log != nil {
		r.log.Infof("Running shell action_id=%d file=%s inventory=%s work_dir=%s\n", request.Action.ID, request.Action.FilePath, request.InventoryPath, workDir)
		r.log.Debugf("Ansible command: %s\n", cmd.String())
	}

	err = executor.Execute(ctx)
	output = AnsibleRunOutput{
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		FactCacheDir: factsDir,
	}

	if err != nil {
		if r.log != nil {
			r.log.Errorf(
				"Shell action failed action_id=%d file=%s error=%v stdout=%s stderr=%s\n",
				request.Action.ID,
				request.Action.FilePath,
				err,
				truncateLog(stdout.String(), 1200),
				truncateLog(stderr.String(), 1200),
			)
		}

		details = combinedCommandOutput(output)
		if details != "" {
			err = fmt.Errorf("%w: %s", err, details)
		}

		err = NewExecutionError("run ansible shell script", err)
		return
	}

	if r.log != nil {
		r.log.Infof("Shell action completed action_id=%d stdout_bytes=%d stderr_bytes=%d\n", request.Action.ID, len(output.Stdout), len(output.Stderr))
	}

	return
}

// playbookOptions maps application configuration to ansible-playbook flags.
func (r *GoAnsibleRunner) playbookOptions(request AnsibleRunRequest) (options *playbook.AnsiblePlaybookOptions) {
	options = &playbook.AnsiblePlaybookOptions{
		Inventory:     strings.TrimSpace(request.InventoryPath),
		Limit:         strings.TrimSpace(request.Limit),
		SSHCommonArgs: strings.TrimSpace(r.config.SSHCommonArgs),
		Timeout:       r.config.TimeoutSeconds,
		User:          strings.TrimSpace(r.config.SSHUser),
		Forks:         strings.TrimSpace(r.config.Forks),
	}
	return
}

// adhocOptions maps application configuration to ansible script-module flags.
func (r *GoAnsibleRunner) adhocOptions(request AnsibleRunRequest, actionPath string) (options *adhoc.AnsibleAdhocOptions) {
	options = &adhoc.AnsibleAdhocOptions{
		Args:          strings.TrimSpace(strings.Join(append([]string{actionPath}, request.Action.Arguments...), " ")),
		Inventory:     strings.TrimSpace(request.InventoryPath),
		Limit:         strings.TrimSpace(request.Limit),
		ModuleName:    "script",
		SSHCommonArgs: strings.TrimSpace(r.config.SSHCommonArgs),
		Timeout:       r.config.TimeoutSeconds,
		User:          strings.TrimSpace(r.config.SSHUser),
		Forks:         strings.TrimSpace(r.config.Forks),
	}
	return
}

// prepareFactsDir ensures the per-run Ansible fact cache directory exists.
func (r *GoAnsibleRunner) prepareFactsDir(workDir string) (factsDir string, err error) {
	factsDir = filepath.Join(workDir, "facts")
	if err = os.MkdirAll(factsDir, 0750); err != nil {
		err = NewExecutionError("create ansible fact cache directory", err)
		return
	}

	return
}

// ansibleEnv returns environment variables used for structured Ansible output.
func (r *GoAnsibleRunner) ansibleEnv(factsDir string) (env map[string]string) {
	env = map[string]string{
		"ANSIBLE_CACHE_PLUGIN":            "jsonfile",
		"ANSIBLE_CACHE_PLUGIN_CONNECTION": factsDir,
		"ANSIBLE_CACHE_PLUGIN_TIMEOUT":    "86400",
	}
	return
}

// prepareWorkDir ensures the Ansible working directory exists.
func (r *GoAnsibleRunner) prepareWorkDir(override string) (workDir string, err error) {
	workDir = strings.TrimSpace(override)
	if workDir == "" {
		workDir = strings.TrimSpace(r.config.WorkDir)
	}

	if workDir == "" {
		workDir = "run/ansible"
	}

	if workDir, err = filepath.Abs(workDir); err != nil {
		err = NewExecutionError("resolve ansible work directory", err)
		return
	}

	if err = os.MkdirAll(workDir, 0750); err != nil {
		err = NewExecutionError("create ansible work directory", err)
		return
	}

	return
}

// validateAnsibleRequest checks fields required before invoking local Ansible binaries.
func validateAnsibleRequest(request AnsibleRunRequest) (err error) {
	if strings.TrimSpace(request.InventoryPath) == "" {
		err = NewInvalidInputError("ansible inventory path is required", nil)
		return
	}

	if strings.TrimSpace(request.Action.FilePath) == "" {
		err = NewInvalidInputError("ansible action file is required", nil)
		return
	}

	return
}

// playbookBinary returns the configured ansible-playbook binary.
func (r *GoAnsibleRunner) playbookBinary() (binary string) {
	binary = strings.TrimSpace(r.config.PlaybookBinary)
	if binary == "" {
		binary = "ansible-playbook"
	}

	return
}

// adhocBinary returns the configured ansible binary.
func (r *GoAnsibleRunner) adhocBinary() (binary string) {
	binary = strings.TrimSpace(r.config.AdhocBinary)
	if binary == "" {
		binary = "ansible"
	}

	return
}

// combinedCommandOutput joins stdout and stderr into a bounded diagnostic string.
func combinedCommandOutput(output AnsibleRunOutput) (details string) {
	var parts []string

	if strings.TrimSpace(output.Stdout) != "" {
		parts = append(parts, "stdout="+truncateLog(output.Stdout, 2400))
	}

	if strings.TrimSpace(output.Stderr) != "" {
		parts = append(parts, "stderr="+truncateLog(output.Stderr, 2400))
	}

	details = strings.Join(parts, " ")
	return
}

// truncateLog keeps command output snippets readable in terminal logs.
func truncateLog(value string, limit int) (truncated string) {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		truncated = value
		return
	}

	truncated = value[:limit] + "... truncated"
	return
}
