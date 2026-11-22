package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateContainerLink(t *testing.T) {
	for _, ok := range []string{"", "https://arcane.example.org/containers/{id}?env=0", "http://10.0.0.2:3552/c/{name}/{project}"} {
		if err := ValidateContainerLink(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"arcane/containers/{id}", "javascript:alert('{id}')", "https://x.example/{container}", "https:///{id}"} {
		if err := ValidateContainerLink(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestContainerLinkIsOnlySentToSignedInUsersWithDocker(t *testing.T) {
	const link = "https://arcane.example.org/containers/{id}?env=0"
	_, watcher := fakeDocker(t)
	auth := NewAuth(true, "pw", "", 0, true)
	s := newTestServer(t, auth, Options{Docker: watcher, ContainerLink: link})
	session := func(s *Server, cookie *http.Cookie) string {
		r := httptest.NewRequest(http.MethodGet, "/api/session", nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		return serve(s, r).Body.String()
	}
	if body := session(s, nil); strings.Contains(body, "arcane") {
		t.Fatalf("anonymous session must not reveal the link: %s", body)
	}
	w := serve(s, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"pw"}`)))
	if body := session(s, w.Result().Cookies()[0]); !strings.Contains(body, `"containerLink":"https://arcane.example.org/containers/{id}?env=0"`) {
		t.Fatalf("signed-in session must include the link: %s", body)
	}
	noDocker := newTestServer(t, NewAuth(false, "", "", 0, true), Options{ContainerLink: link})
	if body := session(noDocker, nil); strings.Contains(body, "containerLink") {
		t.Fatalf("without Docker there is nothing to link: %s", body)
	}
}
