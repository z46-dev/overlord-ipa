package services

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

type DataFileKind string

const (
	DataFileKindOther    DataFileKind = "other"
	DataFileKindPlaybook DataFileKind = "playbook"
	DataFileKindShell    DataFileKind = "shell"
)

type DataFileInfo struct {
	Path       string       `json:"path"`
	Name       string       `json:"name"`
	Kind       DataFileKind `json:"kind"`
	Size       int64        `json:"size"`
	ModifiedAt time.Time    `json:"modified_at"`
	Protected  bool         `json:"protected"`
}

type DataFileContent struct {
	Path       string       `json:"path"`
	Name       string       `json:"name"`
	Kind       DataFileKind `json:"kind"`
	Content    string       `json:"content"`
	ModifiedAt time.Time    `json:"modified_at"`
	Protected  bool         `json:"protected"`
}

type DataFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type DataFileService struct {
	root           string
	protectedFiles map[string]string
}

// NewDataFileService creates a filesystem-backed data directory service.
func NewDataFileService(config conf.DataConfig) (service *DataFileService) {
	service = &DataFileService{
		root: config.Directory,
		protectedFiles: map[string]string{
			"playbooks/health.yml":          defaultHealthPlaybook,
			"playbooks/inventory.yml":       defaultInventoryPlaybook,
			"playbooks/software-update.yml": defaultSoftwareUpdatePlaybook,
		},
	}
	return
}

// EnsureDefaultFiles creates the data directory and protected default playbooks.
func (s *DataFileService) EnsureDefaultFiles(ctx context.Context) (err error) {
	var (
		absolutePath string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if err = os.MkdirAll(s.root, 0750); err != nil {
		err = NewExecutionError("create data directory", err)
		return
	}

	for relativePath, content := range s.protectedFiles {
		if absolutePath, err = s.resolvePath(relativePath); err != nil {
			return
		}

		if err = os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
			err = NewExecutionError("create protected file directory", err)
			return
		}

		if _, err = os.Stat(absolutePath); err == nil {
			continue
		}

		if !os.IsNotExist(err) {
			err = NewExecutionError("stat protected file", err)
			return
		}

		if err = os.WriteFile(absolutePath, []byte(content), 0640); err != nil {
			err = NewExecutionError("write protected file", err)
			return
		}
	}

	return
}

// ListFiles returns data directory files with metadata.
func (s *DataFileService) ListFiles(ctx context.Context) (files []DataFileInfo, err error) {
	var root string

	if err = ctx.Err(); err != nil {
		return
	}

	if err = os.MkdirAll(s.root, 0750); err != nil {
		err = NewExecutionError("create data directory", err)
		return
	}

	if root, err = filepath.Abs(s.root); err != nil {
		err = NewExecutionError("resolve data directory", err)
		return
	}

	files = []DataFileInfo{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) (err error) {
		var (
			info         fs.FileInfo
			relativePath string
		)

		if walkErr != nil {
			err = walkErr
			return
		}

		if entry.IsDir() {
			return
		}

		if info, err = entry.Info(); err != nil {
			return
		}

		if relativePath, err = filepath.Rel(root, path); err != nil {
			return
		}

		relativePath = filepath.ToSlash(relativePath)
		files = append(files, DataFileInfo{
			Path:       relativePath,
			Name:       filepath.Base(relativePath),
			Kind:       dataFileKind(relativePath),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC(),
			Protected:  s.isProtected(relativePath),
		})
		return
	})
	if err != nil {
		err = NewExecutionError("list data files", err)
		return
	}

	slices.SortFunc(files, func(a DataFileInfo, b DataFileInfo) (cmp int) {
		cmp = strings.Compare(a.Path, b.Path)
		return
	})
	return
}

// ReadFile returns a single data file and its content.
func (s *DataFileService) ReadFile(ctx context.Context, path string) (file DataFileContent, err error) {
	var (
		absolutePath string
		content      []byte
		info         fs.FileInfo
		relativePath string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if absolutePath, err = s.resolvePath(path); err != nil {
		return
	}

	if content, err = os.ReadFile(absolutePath); err != nil {
		if os.IsNotExist(err) {
			err = NewNotFoundError("data file not found", err)
			return
		}

		err = NewExecutionError("read data file", err)
		return
	}

	if info, err = os.Stat(absolutePath); err != nil {
		err = NewExecutionError("stat data file", err)
		return
	}

	if relativePath, err = s.normalizePath(path); err != nil {
		return
	}

	file = DataFileContent{
		Path:       relativePath,
		Name:       filepath.Base(relativePath),
		Kind:       dataFileKind(relativePath),
		Content:    string(content),
		ModifiedAt: info.ModTime().UTC(),
		Protected:  s.isProtected(relativePath),
	}
	return
}

// WriteFile creates or updates an editable data file.
func (s *DataFileService) WriteFile(ctx context.Context, input DataFileInput) (file DataFileContent, err error) {
	var (
		absolutePath string
		relativePath string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if relativePath, err = s.normalizePath(input.Path); err != nil {
		return
	}

	if s.isProtected(relativePath) {
		err = NewForbiddenError("protected data files cannot be changed", nil)
		return
	}

	if absolutePath, err = s.resolvePath(relativePath); err != nil {
		return
	}

	if err = os.MkdirAll(filepath.Dir(absolutePath), 0750); err != nil {
		err = NewExecutionError("create data file directory", err)
		return
	}

	if err = os.WriteFile(absolutePath, []byte(input.Content), 0640); err != nil {
		err = NewExecutionError("write data file", err)
		return
	}

	if file, err = s.ReadFile(ctx, relativePath); err != nil {
		return
	}

	return
}

// DeleteFile removes an editable data file.
func (s *DataFileService) DeleteFile(ctx context.Context, path string) (err error) {
	var (
		absolutePath string
		relativePath string
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if relativePath, err = s.normalizePath(path); err != nil {
		return
	}

	if s.isProtected(relativePath) {
		err = NewForbiddenError("protected data files cannot be deleted", nil)
		return
	}

	if absolutePath, err = s.resolvePath(relativePath); err != nil {
		return
	}

	if err = os.Remove(absolutePath); err != nil {
		if os.IsNotExist(err) {
			err = NewNotFoundError("data file not found", err)
			return
		}

		err = NewExecutionError("delete data file", err)
		return
	}

	return
}

// ValidateActionFile ensures an action references an allowed data file.
func (s *DataFileService) ValidateActionFile(ctx context.Context, path string, actionType db.JobActionType) (err error) {
	var (
		absolutePath string
		kind         DataFileKind
		info         fs.FileInfo
	)

	if err = ctx.Err(); err != nil {
		return
	}

	if absolutePath, err = s.resolvePath(path); err != nil {
		return
	}

	if info, err = os.Stat(absolutePath); err != nil {
		if os.IsNotExist(err) {
			err = NewInvalidInputError("action file does not exist in data directory", err)
			return
		}

		err = NewExecutionError("stat action file", err)
		return
	}

	if info.IsDir() {
		err = NewInvalidInputError("action file must be a file", nil)
		return
	}

	kind = dataFileKind(path)
	switch actionType {
	case db.JobActionTypeAnsiblePlaybook:
		if kind != DataFileKindPlaybook {
			err = NewInvalidInputError("ansible actions require a playbook file", nil)
			return
		}
	case db.JobActionTypeShell:
		if kind != DataFileKindShell {
			err = NewInvalidInputError("shell actions require a shell script file", nil)
			return
		}
	default:
		err = NewInvalidInputError("unsupported action type", nil)
	}

	return
}

// normalizePath validates a relative data file path.
func (s *DataFileService) normalizePath(path string) (normalized string, err error) {
	normalized = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	normalized = strings.TrimPrefix(normalized, "./")

	if normalized == "" || normalized == "." {
		err = NewInvalidInputError("data file path is required", nil)
		return
	}

	if filepath.IsAbs(normalized) || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") || normalized == ".." {
		err = NewInvalidInputError("data file path must stay inside the data directory", nil)
		return
	}

	return
}

// resolvePath returns an absolute path under the configured data directory.
func (s *DataFileService) resolvePath(path string) (absolutePath string, err error) {
	var (
		root         string
		relativePath string
	)

	if relativePath, err = s.normalizePath(path); err != nil {
		return
	}

	if root, err = filepath.Abs(s.root); err != nil {
		err = NewExecutionError("resolve data directory", err)
		return
	}

	absolutePath = filepath.Join(root, filepath.FromSlash(relativePath))
	if !strings.HasPrefix(absolutePath, root+string(os.PathSeparator)) && absolutePath != root {
		err = NewInvalidInputError("data file path must stay inside the data directory", nil)
		return
	}

	return
}

// isProtected reports whether a path is one of the built-in data files.
func (s *DataFileService) isProtected(path string) (protected bool) {
	var relativePath string
	relativePath, _ = s.normalizePath(path)
	_, protected = s.protectedFiles[relativePath]
	return
}

// dataFileKind classifies files by extension for action binding.
func dataFileKind(path string) (kind DataFileKind) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml":
		kind = DataFileKindPlaybook
	case ".sh", ".bash":
		kind = DataFileKindShell
	default:
		kind = DataFileKindOther
	}

	return
}

const defaultHealthPlaybook string = `---
- name: Overlord IPA health check
  hosts: all
  gather_facts: false
  tasks:
    - name: Check SSH connectivity
      ansible.builtin.ping:

    - name: Poll uptime
      ansible.builtin.command: uptime
      changed_when: false

    - name: Poll logged in users
      ansible.builtin.command: who
      changed_when: false

    - name: Poll disk usage
      ansible.builtin.command: df -h
      changed_when: false
`

const defaultInventoryPlaybook string = `---
- name: Overlord IPA inventory collection
  hosts: all
  gather_facts: true
  tasks:
    - name: Collect hardware summary
      ansible.builtin.setup:
        gather_subset:
          - hardware
          - network
          - virtual

    - name: Collect operating system release
      ansible.builtin.command: cat /etc/os-release
      changed_when: false
`

const defaultSoftwareUpdatePlaybook string = `---
- name: Overlord IPA software update
  hosts: all
  become: true
  gather_facts: true
  tasks:
    - name: Update DNF based systems
      ansible.builtin.dnf:
        name: "*"
        state: latest
      when: ansible_facts.pkg_mgr == "dnf"

    - name: Update APT based systems
      ansible.builtin.apt:
        upgrade: dist
        update_cache: true
      when: ansible_facts.pkg_mgr == "apt"
`
