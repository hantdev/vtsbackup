package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hantbk/vtsbackup/helper"
	"github.com/hantbk/vtsbackup/logger"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

var (
	// Exist Is config file exist
	Exist bool
	// Models configs
	Models []ModelConfig
	// backup base dir
	VtsBackupDir string = getBackupDir()

	PidFilePath string = filepath.Join(VtsBackupDir, "vtsbackup.pid")
	LogFilePath string = filepath.Join(VtsBackupDir, "vtsbackup.log")
	Web         WebConfig

	wLock = sync.Mutex{}

	// The config file loaded at
	UpdatedAt time.Time

	onConfigChanges = make([]func(fsnotify.Event), 0)

	// Backup server
	BackupDir               string
	BackupList              []string
	BackupListExcluded      []string
	BackupIptables          int
	BackupCrontabs          int
	CheckUsers              int
	CheckEnabledServices    int
	CheckInstalledPackages  int
	MaintainFilePermissions int
)

type WebConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

type ScheduleConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	// Cron expression
	Cron string `json:"cron,omitempty"`
	// Every
	Every string `json:"every,omitempty"`
	// At time
	At string `json:"at,omitempty"`
}

func (sc ScheduleConfig) String() string {
	if sc.Enabled {
		if len(sc.Cron) > 0 {
			return fmt.Sprintf("cron %s", sc.Cron)
		} else {
			if len(sc.At) > 0 {
				return fmt.Sprintf("every %s at %s", sc.Every, sc.At)
			} else {
				return fmt.Sprintf("every %s", sc.Every)
			}
		}
	}

	return "disabled"
}

// ModelConfig for special case
type ModelConfig struct {
	Name        string
	Description string
	// WorkDir of the gobackup started
	WorkDir        string
	TempPath       string
	DumpPath       string
	Schedule       ScheduleConfig
	CompressWith   SubConfig
	EncryptWith    SubConfig
	Archive        *viper.Viper
	Splitter       *viper.Viper
	Storages       map[string]SubConfig
	DefaultStorage string
	Notifiers      map[string]SubConfig
	Viper          *viper.Viper
	BeforeScript   string
	AfterScript    string
}

func getBackupDir() string {
	dir := os.Getenv("BACKUP_DIR")
	if len(dir) == 0 {
		dir = filepath.Join(os.Getenv("HOME"), ".vtsbackup")
	}
	return dir
}

// SubConfig sub config info
type SubConfig struct {
	Name  string
	Type  string
	Viper *viper.Viper
}

// Init
// loadConfig from:
// - ./vtsbackup.yml
// - ~/.vtsbackup/vtsbackup.yml
// - /etc/vtsbackup/vtsbackup.yml
func Init(configFile string) error {
	logger := logger.Tag("Config")

	viper.SetConfigType("yaml")

	// set config file directly
	if len(configFile) > 0 {
		configFile = helper.AbsolutePath(configFile)
		logger.Info("Load config:", configFile)

		viper.SetConfigFile(configFile)
	} else {
		logger.Info("Load config from default path.")
		viper.SetConfigName("vtsbackup")

		// ./vtsbackup.yml
		viper.AddConfigPath(".")
		// ~/.vtsbackup/vtsbackup.yml
		viper.AddConfigPath("$HOME/.vtsbackup") // call multiple times to add many search paths
		// /etc/vtsbackup/vtsbackup.yml
		viper.AddConfigPath("/etc/vtsbackup/") // path to look for the config file in
	}

	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		logger.Info("Config file changed:", in.Name)
		defer onConfigChanged(in)
		if err := loadConfig(); err != nil {
			logger.Error(err.Error())
		}
	})

	return loadConfig()
}

// OnConfigChange add callback when config changed
func OnConfigChange(run func(in fsnotify.Event)) {
	onConfigChanges = append(onConfigChanges, run)
}

// Invoke callbacks when config changed
func onConfigChanged(in fsnotify.Event) {
	for _, fn := range onConfigChanges {
		fn(in)
	}
}

func loadConfig() error {
	wLock.Lock()
	defer wLock.Unlock()

	logger := logger.Tag("Config")

	err := viper.ReadInConfig()
	if err != nil {
		logger.Error("Load backup config failed: ", err)
		return err
	}

	viperConfigFile := viper.ConfigFileUsed()
	if info, err := os.Stat(viperConfigFile); err == nil {
		// max permission: 0770
		if info.Mode()&(1<<2) != 0 {
			logger.Warnf("Other users are able to access %s with mode %v", viperConfigFile, info.Mode())
		}
	}

	logger.Info("Config file:", viperConfigFile)

	// load .env if exists in the same directory of used config file and expand variables in the config
	dotEnv := filepath.Join(filepath.Dir(viperConfigFile), ".env")
	if _, err := os.Stat(dotEnv); err == nil {
		if err := godotenv.Load(dotEnv); err != nil {
			logger.Errorf("Load %s failed: %v", dotEnv, err)
			return err
		}
	}

	cfg, _ := os.ReadFile(viperConfigFile)
	if err := viper.ReadConfig(strings.NewReader(os.ExpandEnv(string(cfg)))); err != nil {
		logger.Errorf("Load expanded config failed: %v", err)
		return err
	}

	// TODO: Here the `useTempWorkDir` and `workdir`, is not in config document. We need removed it.
	viper.Set("useTempWorkDir", false)
	if workdir := viper.GetString("workdir"); len(workdir) == 0 {
		// use temp dir as workdir
		dir, err := os.MkdirTemp("", "backup")
		if err != nil {
			return err
		}

		viper.Set("workdir", dir)
		viper.Set("useTempWorkDir", true)
	}

	Exist = true
	Models = []ModelConfig{}
	for key := range viper.GetStringMap("models") {
		model, err := loadModel(key)
		if err != nil {
			return fmt.Errorf("load model %s: %v", key, err)
		}

		Models = append(Models, model)
	}

	if len(Models) == 0 {
		return fmt.Errorf("no model found in %s", viperConfigFile)
	}

	// Load web config
	Web = WebConfig{}
	viper.SetDefault("web.host", "0.0.0.0")
	viper.SetDefault("web.port", 2703)
	Web.Host = viper.GetString("web.host")
	Web.Port = viper.GetString("web.port")
	Web.Username = viper.GetString("web.username")
	Web.Password = viper.GetString("web.password")

	UpdatedAt = time.Now()
	logger.Infof("Config loaded, found %d models.", len(Models))

	// Load backup server config
	BackupDir = viper.GetString("backupDir")
	BackupList = viper.GetStringSlice("filesConfig.backupList")
	BackupListExcluded = viper.GetStringSlice("filesConfig.BackupListExcluded")
	BackupIptables = viper.GetInt("servicesConfig.backupIptables")
	BackupCrontabs = viper.GetInt("servicesConfig.backupCrontabs")
	CheckUsers = viper.GetInt("servicesConfig.checkUsers")
	CheckEnabledServices = viper.GetInt("servicesConfig.checkEnabledServices")
	CheckInstalledPackages = viper.GetInt("servicesConfig.checkInstalledPackages")
	MaintainFilePermissions = viper.GetInt("extraConfig.maintainFilePermissions")

	return nil
}

func loadModel(key string) (ModelConfig, error) {
	var model ModelConfig
	model.Name = key

	workdir, _ := os.Getwd()

	model.WorkDir = workdir
	model.TempPath = filepath.Join(viper.GetString("workdir"), fmt.Sprintf("%d", time.Now().UnixNano()))
	model.DumpPath = filepath.Join(model.TempPath, key)
	model.Viper = viper.Sub("models." + key)

	model.Description = model.Viper.GetString("description")
	model.Schedule = ScheduleConfig{Enabled: false}

	model.CompressWith = SubConfig{
		Type:  model.Viper.GetString("compress_with.type"),
		Viper: model.Viper.Sub("compress_with"),
	}

	model.EncryptWith = SubConfig{
		Type:  model.Viper.GetString("encrypt_with.type"),
		Viper: model.Viper.Sub("encrypt_with"),
	}

	model.Archive = model.Viper.Sub("archive")
	model.Splitter = model.Viper.Sub("split_with")

	model.BeforeScript = model.Viper.GetString("before_script")
	model.AfterScript = model.Viper.GetString("after_script")

	loadScheduleConfig(&model)
	loadStoragesConfig(&model)

	if len(model.Storages) == 0 {
		return ModelConfig{}, fmt.Errorf("no storage found in model %s", model.Name)
	}

	return model, nil
}

func loadScheduleConfig(model *ModelConfig) {
	subViper := model.Viper.Sub("schedule")
	model.Schedule = ScheduleConfig{Enabled: false}
	if subViper == nil {
		return
	}

	model.Schedule = ScheduleConfig{
		Enabled: true,
		Cron:    subViper.GetString("cron"),
		Every:   subViper.GetString("every"),
		At:      subViper.GetString("at"),
	}
}

func loadStoragesConfig(model *ModelConfig) {
	storageConfigs := map[string]SubConfig{}

	model.DefaultStorage = model.Viper.GetString("default_storage")

	subViper := model.Viper.Sub("storages")
	for key := range model.Viper.GetStringMap("storages") {
		storageViper := subViper.Sub(key)
		storageConfigs[key] = SubConfig{
			Name:  key,
			Type:  storageViper.GetString("type"),
			Viper: storageViper,
		}

		// Set default storage
		if len(model.DefaultStorage) == 0 {
			model.DefaultStorage = key
		}
	}
	model.Storages = storageConfigs

}

// GetModelConfigByName get model config by name
func GetModelConfigByName(name string) (model *ModelConfig) {
	for _, m := range Models {
		if m.Name == name {
			model = &m
			return
		}
	}
	return
}
