package types

import (
	"net"
	"strings"

	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

func generateContainerID() string {
	const randomLen = 8

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return fn.RandNChars(randomLen)
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}

		ip := ipNet.IP.To4()
		if ip == nil {
			continue
		}

		return strings.ReplaceAll(ip.String(), ".", "-") + "-" + fn.RandNChars(randomLen)
	}

	return fn.RandNChars(randomLen)
}
