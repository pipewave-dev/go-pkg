package configprovider

import types "github.com/pipewave-dev/go-pkg/export/types"

func FromGoStruct(input types.EnvType) ConfigStore {
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
