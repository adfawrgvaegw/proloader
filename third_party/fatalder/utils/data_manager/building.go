package data_manager

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BuildingFileInfo describes a structure file stored on disk.
type BuildingFileInfo struct {
	Name    string
	Size    int64
	MD5     string
	ModTime time.Time
}

// ListBuildingFiles returns metadata for every stored building file.
func (dm *DataManager) ListBuildingFiles() ([]BuildingFileInfo, error) {
	dm.buildingMu.RLock()
	defer dm.buildingMu.RUnlock()

	entries, err := os.ReadDir(dm.buildingsDirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	result := make([]BuildingFileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".upload") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		path := filepath.Join(dm.buildingsDirPath, name)
		md5Sum, err := calculateFileMD5(path)
		if err != nil {
			return nil, err
		}

		result = append(result, BuildingFileInfo{
			Name:    name,
			Size:    info.Size(),
			MD5:     md5Sum,
			ModTime: info.ModTime(),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

// GetBuildingInfo returns metadata for a single building file.
func (dm *DataManager) GetBuildingInfo(filename string) (*BuildingFileInfo, bool, error) {
	cleanName, err := sanitizeBuildingFilename(filename)
	if err != nil {
		return nil, false, err
	}

	dm.buildingMu.RLock()
	defer dm.buildingMu.RUnlock()

	path := filepath.Join(dm.buildingsDirPath, cleanName)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	md5Sum, err := calculateFileMD5(path)
	if err != nil {
		return nil, false, err
	}

	return &BuildingFileInfo{
		Name:    cleanName,
		Size:    info.Size(),
		MD5:     md5Sum,
		ModTime: info.ModTime(),
	}, true, nil
}

// DeleteBuilding removes a building file from storage.
func (dm *DataManager) DeleteBuilding(filename string) error {
	cleanName, err := sanitizeBuildingFilename(filename)
	if err != nil {
		return err
	}

	dm.buildingMu.Lock()
	defer dm.buildingMu.Unlock()

	return os.Remove(filepath.Join(dm.buildingsDirPath, cleanName))
}

// PrepareBuildingFilePath returns an absolute path inside the building storage directory.
func (dm *DataManager) PrepareBuildingFilePath(filename string) (string, error) {
	cleanName, err := sanitizeBuildingFilename(filename)
	if err != nil {
		return "", err
	}

	dm.buildingMu.Lock()
	defer dm.buildingMu.Unlock()

	if err := os.MkdirAll(dm.buildingsDirPath, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dm.buildingsDirPath, cleanName), nil
}

func sanitizeBuildingFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("filename is required")
	}
	if name == "." || name == ".." {
		return "", errors.New("invalid filename")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("filename %q must not contain path separators", name)
	}
	return name, nil
}

func calculateFileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
