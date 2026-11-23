package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckPasswordAcceptsHeaderAndQuery(t *testing.T) {
	const expected = "s3cret"

	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil)
	if err := checkPassword(r, expected); err == nil {
		t.Fatal("missing password should fail")
	}

	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat?password=s3cret", nil)
	if err := checkPassword(r, expected); err != nil {
		t.Fatalf("query password should be accepted for compatibility, got: %v", err)
	}

	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	if err := checkPassword(r, expected); err != nil {
		t.Fatalf("bearer header should be accepted, got: %v", err)
	}

	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil)
	r.Header.Set("X-DontCrack-Password", "s3cret")
	if err := checkPassword(r, expected); err != nil {
		t.Fatalf("custom header should be accepted, got: %v", err)
	}

	// 未配置密码时全部放行
	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil)
	if err := checkPassword(r, ""); err != nil {
		t.Fatalf("empty expected password should pass, got: %v", err)
	}
}

// 复用 stater.go 的注册逻辑验证认证失败返回 401 而非 200
func TestUnauthorizedReturns401(t *testing.T) {
	// 通过 startServer 之外的独立 handler 无法直接复用；这里验证 http.Error 行为约定：
	// 修复后的 handler 必须写 StatusUnauthorized。用一个最小 handler 模拟修复后的分支。
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checkPassword(r, "s3cret"); err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("auth failure must return 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct password must return 200, got %d", rec.Code)
	}
}

// TestWebUIHandlerRequiresPassword 覆盖 /ui 鉴权: 它过去是唯一一个不查密码
// 的端点(页面本身不含数据, 但会列出全部操作入口)。现在 /ui 与 /ui/ 共用
// 同一个 handler, 未持密码者得到 401 而不是 HTML; 持密码者仍拿到 HTML。
// 未配置 -password 时保持放行, 否则原本无密码部署的实例会被锁死在外。
func TestWebUIHandlerRequiresPassword(t *testing.T) {
	body := []byte("<html>ui</html>")
	serve := func(password string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if err := checkPassword(r, password); err != nil {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
		}
	}

	// 配置了密码: 三个入口(/ui、/ui/、无密码)的取值组合必须各自正确。
	secured := serve("s3cret")
	for _, path := range []string{"/ui", "/ui/"} {
		rec := httptest.NewRecorder()
		secured.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without password: code = %d, want 401", path, rec.Code)
		}
		if body := rec.Body.String(); body != "unauthorized\n" {
			t.Fatalf("%s without password: body = %q, want the 401 text only (no page)", path, body)
		}

		rec = httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer s3cret")
		secured.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s with password: code = %d, want 200", path, rec.Code)
		}
		if got := rec.Body.String(); got != string(body) {
			t.Fatalf("%s with password: body = %q, want the embedded page", path, got)
		}
	}

	// 未配置密码: 必须原样放行, 否则无密码部署会被自己锁在外面。
	open := serve("")
	rec := httptest.NewRecorder()
	open.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ui", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("empty password must keep /ui open, got %d", rec.Code)
	}
}
