package restapi

import (
	"encoding/json"
	"net/http"

	"github.com/pipewave-dev/go-pkg/shared/aerror"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errBody struct {
	Error errDetail `json:"error"`
}

type errDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeAError(w http.ResponseWriter, aErr aerror.AError) {
	code := aErr.ErrorCode()
	writeJSON(w, code.HttpCode(), errBody{Error: errDetail{Code: code.String(), Message: aErr.Error()}})
}

func writeBadRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errBody{Error: errDetail{Code: aerror.ErrInvalidInput.String(), Message: msg}})
}

func writeUnauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, errBody{Error: errDetail{Code: aerror.LogicErrMissingAuthHeader.String(), Message: "missing or invalid API key"}})
}

// decodeBody strictly decodes the JSON request body into dst.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeBadRequest(w, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
