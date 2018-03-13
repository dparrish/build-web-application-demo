package swagger

import (
	"io/ioutil"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorfSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	Errorf(w, 200, "OK %s", "world")
	res := w.Result()
	assert.Equal(t, res.StatusCode, 200)
	b, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, strings.TrimSpace(string(b)), `{"code":200,"message":"OK world"}`)
}

func TestErrorfFailure(t *testing.T) {
	w := httptest.NewRecorder()
	Errorf(w, 404, "not found")
	res := w.Result()
	assert.Equal(t, res.StatusCode, 404)
	b, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, strings.TrimSpace(string(b)), `{"code":404,"message":"not found"}`)
}
