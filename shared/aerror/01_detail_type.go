/*
This is just an example
*/

package aerror

type DetailAllowType interface {
	SenderInfo |
		ValidationDetail
}

type SenderInfo struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type (
	ValidationDetail       []ValidationDetailInside
	ValidationDetailInside struct {
		Key      string `json:"key"`
		ValueStr string `json:"value"`
		Message  string `json:"message"`
	}
)
