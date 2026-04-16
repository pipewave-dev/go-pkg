package configprovider

import (
	koanfpvd "github.com/pipewave-dev/go-pkg/pkg/koanf"
	types "github.com/pipewave-dev/go-pkg/sdk/types"
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
func FromYaml(yamlFiles []string) ConfigStore {
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

	var input types.EnvType
	k.Unmarshall(&input)

	// Load default values and validate
	input.LoadDefault()
	input.Validate()

	env := globalEnvT{
		EnvType: input,
		Fns:     nil,
	}
	env.LoadDefault()
	env.Validate()

	return &configStore{
		env: &env,
	}
}
