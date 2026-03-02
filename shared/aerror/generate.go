package aerror

//go:generate go tool golang.org/x/tools/cmd/stringer -type=ErrorCode

//go:generate go run ../../tools/gen-template-for-enum -path=. -type=ErrorCode -output=./errorcode_i18n.g.go -tmpl_file=./_errorcode.go.tmpl
