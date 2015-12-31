package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
)

// Store provides Machine, Spec, and config resources.
type Store interface {
	Machine(id string) (*Machine, error)
	Spec(id string) (*Spec, error)
	// CloudConfig returns the cloud config user data for the machine.
	CloudConfig(attrs MachineAttrs) (*CloudConfig, error)
}

// fileStore maps machine attributes to configs based on an http.Filesystem.
type fileStore struct {
	root http.FileSystem
}

// NewFileStore returns a Store backed by a filesystem directory.
func NewFileStore(root http.FileSystem) Store {
	return &fileStore{
		root: root,
	}
}

const (
	bootPrefix  = "boot"
	cloudPrefix = "cloud"
)

// Machine returns the configuration for the machine with the given id.
func (s *fileStore) Machine(id string) (*Machine, error) {
	file, err := openFile(s.root, filepath.Join("machines", id, "machine.json"))
	if err != nil {
		log.Infof("no machine config %s", id)
		return nil, err
	}
	defer file.Close()

	machine := new(Machine)
	err = json.NewDecoder(file).Decode(machine)
	if err != nil {
		log.Errorf("error decoding machine config: %s", err)
	}

	if machine.BootConfig == nil && machine.SpecID != "" {
		// machine references a Spec, attempt to add Spec properties
		spec, err := s.Spec(machine.SpecID)
		if err == nil {
			machine.BootConfig = spec.BootConfig
		}
	}
	return machine, err
}

// Spec returns the Spec with the given id.
func (s *fileStore) Spec(id string) (*Spec, error) {
	file, err := openFile(s.root, filepath.Join("specs", id, "spec.json"))
	if err != nil {
		log.Infof("no spec %s", id)
		return nil, err
	}
	defer file.Close()

	spec := new(Spec)
	err = json.NewDecoder(file).Decode(spec)
	if err != nil {
		log.Errorf("error decoding spec: %s", err)
	}
	return spec, err
}

// CloudConfig returns the cloud config for the machine.
func (s *fileStore) CloudConfig(attrs MachineAttrs) (*CloudConfig, error) {
	file, err := s.find(cloudPrefix, attrs)
	if err != nil {
		log.Debugf("no cloud config for machine %+v", attrs)
		return nil, err
	}
	defer file.Close()
	// cloudinit requires reading the entire file
	b, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return &CloudConfig{
		Content: string(b),
	}, nil
}

// find searches the prefix subdirectory of root for the first config file
// which matches the given machine attributes. If the error is non-nil, the
// caller must be sure to close the matched http.File. Matches are searched
// in priority order: uuid/<UUID>, mac/<MAC aaddress>, default.
func (s *fileStore) find(prefix string, attrs MachineAttrs) (http.File, error) {
	search := []string{
		filepath.Join("uuid", attrs.UUID),
		filepath.Join("mac", attrs.MAC.String()),
		"/default",
	}
	for _, path := range filter(search) {
		fullPath := filepath.Join(prefix, path)
		if file, err := openFile(s.root, fullPath); err == nil {
			return file, err
		}
	}
	return nil, fmt.Errorf("no %s config for machine %+v", prefix, attrs)
}

// filter returns only paths which have non-empty directory paths. For example,
// "uuid/123" has a directory path "uuid", while path "uuid" does not.
func filter(inputs []string) (paths []string) {
	for _, path := range inputs {
		if filepath.Dir(path) != "." {
			paths = append(paths, path)
		}
	}
	return paths
}

// openFile attempts to open the file within the specified Filesystem. If
// successful, the http.File is returned and must be closed by the caller.
// Otherwise, the path was not a regular file that could be opened and an
// error is returned.
func openFile(fs http.FileSystem, path string) (http.File, error) {
	file, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.Mode().IsRegular() {
		return file, nil
	}
	file.Close()
	return nil, fmt.Errorf("%s is not a file on the given filesystem", path)
}