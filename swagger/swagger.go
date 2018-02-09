package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Errorf writes a swagger-compliant error response.
func Errorf(w http.ResponseWriter, code int, format string, a ...interface{}) {
	out := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{
		Code:    code,
		Message: fmt.Sprintf(format, a...),
	}

	b, err := json.Marshal(out)
	if err != nil {
		http.Error(w, `{"code": 500, "message": "Could not format JSON for original message."}`, 500)
		return
	}

	http.Error(w, string(b), code)
}
