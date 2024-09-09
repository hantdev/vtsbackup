package config

import (
	"fmt"
	"github.com/hantbk/vts-backup/logger"
	"github.com/spf13/viper"
	"os"
	"path"
	"time"
)

var (
	// Models configs
	Models []ModelConfig
)

// ModelConfig for special case
type ModelConfig struct {
	Name         string
	DumpPath     string
	CompressWith SubConfig
	StoreWith    SubConfig
	Storages     []SubConfig
	Viper        *viper.Viper
}

// Subconfig sub config info
type SubConfig struct {
	Name  string
	Type  string
	Viper *viper.Viper
}

func init() {
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	// /etc/vts-backup/config.yml
	viper.AddConfigPath("/etc/vts-backup")
	// ~/.vts-backup/config.yml
	viper.AddConfigPath("$HOME/.vts-backup")

	// ./config.yml
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		logger.Error("Failed to read config file: ", err)
		return
	}

	Models = []ModelConfig{}
	for key := range viper.GetStringMap("models") {

		Models = append(Models, loadModel(key))
	}

	return
}

func loadModel(key string) (model ModelConfig) {
	model.Name = key
	model.DumpPath = path.Join(os.TempDir(), "vts-backup", fmt.Sprintf("%d", time.Now().UnixNano()))
	model.Viper = viper.Sub("models." + key)

	model.CompressWith = SubConfig{
		Type:  model.Viper.GetString("compress_with.type"),
		Viper: model.Viper.Sub("compress_with"),
	}

	model.StoreWith = SubConfig{
		Type:  model.Viper.GetString("store_with.type"),
		Viper: model.Viper.Sub("store_with"),
	}

	loadStoragesConfig(&model)

	return
}

func loadStoragesConfig(model *ModelConfig) {
	subViper := model.Viper.Sub("storages")
	for key := range model.Viper.GetStringMap("storages") {
		dbViper := subViper.Sub(key)
		model.Storages = append(model.Storages, SubConfig{
			Name:  key,
			Type:  dbViper.GetString("type"),
			Viper: dbViper,
		})
	}
}
