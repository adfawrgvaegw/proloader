package data_manager

import (
	"path/filepath"
	"sync"
	"github.com/Yeah114/Fatalder/utils"
)

type DataManager struct {
	storagePath             string
	serverConfigsDirPath    string
	tasksConfigsDirPath     string
	interruptConfigsDirPath string
	buildingsDirPath        string
	serverConfigMu          sync.RWMutex
	tasksConfigMu           sync.RWMutex
	interruptConfigMu       sync.RWMutex
	buildingMu              sync.RWMutex
}

func NewDataManager(path string) *DataManager {
	return &DataManager{
		storagePath:             path,
		serverConfigsDirPath:    filepath.Join(path, utils.ServerConfigsDir),
		tasksConfigsDirPath:     filepath.Join(path, utils.TasksConfigsDir),
		interruptConfigsDirPath: filepath.Join(path, utils.InterruptConfigsDir),
		buildingsDirPath:        filepath.Join(path, utils.BuildingFilesDir),
	}
}

var GlobalDataManager = NewDataManager(utils.GetStoragePath())
