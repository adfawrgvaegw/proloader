package mpay

import "fmt"

// NeedVerifyError 表示 MPay 返回的需要额外验证的错误。
type NeedVerifyError struct {
	Code      int
	Reason    string
	VerifyURL string
}

func (e *NeedVerifyError) Error() string {
	if e == nil {
		return "mpay: 未知验证错误"
	}
	if e.VerifyURL != "" {
		return fmt.Sprintf("mpay: 需要验证 code=%d reason=%s verify_url=%s", e.Code, e.Reason, e.VerifyURL)
	}
	return fmt.Sprintf("mpay: 需要验证 code=%d reason=%s", e.Code, e.Reason)
}
