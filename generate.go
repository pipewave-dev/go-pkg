package pipewave

//go:generate go run ./tools/wire-collect -path=. -outputDir=./gen/wire/ -gen-package=wirecollection
//go:generate go tool github.com/google/wire/cmd/wire gen ./app
//go:generate go generate ./shared/aerror
