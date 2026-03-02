package configprovider

import (
	koanfpvd "github.com/pipewave-dev/go-pkg/pkg/koanf"
)

// FromYaml creates a new ConfigStore instance by loading configuration from YAML files
// and merging with environment variables.
//
// Parameters:
//   - yamlFiles: List of YAML file paths (full path or relative path) to load in order.
//     Later files will override earlier ones.
//   - fnStore: Function store containing injectable functions. Must not be nil.
//
// The function will panic if:
//   - Configuration validation fails
//   - Required YAML files cannot be loaded
//   - fnStore is nil
func FromYaml(yamlFiles []string, fnStore *Fns) ConfigStore {
	if fnStore == nil {
		panic("fnStore must not be nil")
	}

	// Convert yamlFiles to koanf format
	koanfYamlFiles := make([]struct {
		FileDir   string
		FilePath  string
		SkipError bool
	}, 0, len(yamlFiles))

	for _, filePath := range yamlFiles {
		koanfYamlFiles = append(koanfYamlFiles, struct {
			FileDir   string
			FilePath  string
			SkipError bool
		}{
			FileDir:   "", // Empty means use the filePath as is
			FilePath:  filePath,
			SkipError: false, // All provided files are required
		})
	}

	// Load configuration using koanf
	k := koanfpvd.NewKoanfProvider(&koanfpvd.KoanfConfig{
		YamlConfigFile: koanfYamlFiles,
		EnvPrefix:      "APP",
	})

	var env globalEnvT
	k.Unmarshall(&env)

	// Inject function store
	env.Fns = fnStore

	// Load default values and validate
	env.loadDefault()
	env.validate()

	return &configStore{
		env: &env,
	}
}
