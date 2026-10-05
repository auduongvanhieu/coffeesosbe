package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type loginBody struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// The validator's own text is developer noise; clients must get Vietnamese.
func TestBindMessagesAreHumanVietnamese(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct{ body, want string }{
		{`{"email":"not-an-email","password":"DailyBean123"}`, "email không hợp lệ"},
		{`{"password":"DailyBean123"}`, "thiếu email"},
		{`{"email":"a@b.co","password":"123"}`, "mật khẩu phải có ít nhất 6 ký tự"},
		{`not json`, "dữ liệu gửi lên không hợp lệ"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		var dst loginBody
		if Bind(c, &dst) {
			t.Fatalf("Bind(%s) should have failed", tc.body)
		}
		if got := w.Body.String(); !strings.Contains(got, tc.want) {
			t.Errorf("Bind(%s) = %s, want it to contain %q", tc.body, got, tc.want)
		}
		if strings.Contains(w.Body.String(), "Key:") {
			t.Errorf("Bind(%s) leaked the validator message: %s", tc.body, w.Body.String())
		}
	}
}
