package koanf

import (
	"log"
	"os"
	"path"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
)

type KoanfConfig struct {
	/* YamlConfigFile
	Higher priority to latter
	Only support YAML currently
	Reason: lightweight, remove redundances parser package) */
	YamlConfigFile []struct {
		// FileDir Yaml config files directory (default is pwd)
		FileDir   string
		FilePath  string
		SkipError bool
	}

	// EnvExpand expand environment variables in the config values. Default is false.
	// Example
	// If EnvExpand is true, the value of "path" in the config file can be something like "${HOME}/config.yaml", and it will be expanded to the actual path of the config file.
	EnvExpand bool

	/* EnvPrefix
	Load environment variables and merge into the loaded config with the HIGHEST priority.

	For example
	EnvPrefix = "MYVAR" is the prefix to filter the env vars.
	MYVAR_TYPE env var is fetched with tag `koanf:"TYPE"`
	For nested tag. Double underscore (__) is used instead dot (.) because can't use dot in var name.
	MYVAR_LOG__LEVEL env var is fetched with tag `koanf:"LOG.LEVEL"`
	*/
	EnvPrefix string

	/* 	Delimiter
	Delimiter for sub key of yaml file.
	Default dot (.)
	*/
	Delimiter *string
}

type KoanfProvider interface {
	/*
		How to use

			type GlobalEnvT struct {
				SomeInt    int    `koanf:"SOME_INT"`
				Env        string `koanf:"ENV"`
				PartyProps struct {
					Name     *string           `koanf:"NAME"`
					ID       *int32            `koanf:"ID"`
					Channels []string          `koanf:"CHANNELS"`
					Tags     map[string]string `koanf:"TAGS"`
					Tag2s    map[string][]int    `koanf:"TAG2S"`
				} `koanf:"PARTY_PROPS"`
			}

			var (
				GlobalEnv GlobalEnvT
			)

			instance.Unmarshall(&GlobalEnv)
	*/
	Unmarshall(output any)
}

type koanfProvider struct {
	koanfCfg *KoanfConfig
	koanf    *koanf.Koanf
}

func NewKoanfProvider(config *KoanfConfig) KoanfProvider {
	instance := &koanfProvider{
		koanfCfg: config,
	}
	instance.load()
	return instance
}

const delim = "."

func (k *koanfProvider) load() {
	k.koanf = koanf.New(delim)

	// Load YAML config.
	for _, yamlFile := range k.koanfCfg.YamlConfigFile {
		fileDir := "."
		if yamlFile.FileDir != "" {
			fileDir = yamlFile.FileDir
		}

		abspath := path.Join(fileDir, yamlFile.FilePath)
		raw, _ := os.ReadFile(abspath)
		if k.koanfCfg.EnvExpand {
			raw = []byte(os.ExpandEnv(string(raw)))
		}
		err := k.koanf.Load(rawbytes.Provider(raw), yaml.Parser())
		if err != nil && !yamlFile.SkipError {
			log.Panicf("fail load config [%s]: %v", abspath, err)
		}
	}

	delimiter := "__"
	if k.koanfCfg.Delimiter != nil {
		delimiter = *k.koanfCfg.Delimiter
	}
	// Load environment vars and merge at last
	k.koanf.Load(env.Provider(k.koanfCfg.EnvPrefix, delim, func(s string) string {
		return strings.Replace(strings.ToUpper(
			strings.TrimPrefix(s, k.koanfCfg.EnvPrefix)),
			delimiter, ".", -1)
	}), nil)
}
