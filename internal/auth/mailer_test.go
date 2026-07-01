package auth

import (
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

// TestSMTPMailerMessageNoLogo asserts that without a logo the OTP email is a
// multipart/alternative carrying both a text/plain and text/html part with the
// code, and no image part.
func TestSMTPMailerMessageNoLogo(t *testing.T) {
	m := NewSMTPMailer("smtp.example.com", "587", "user", "pass", "login@unbusy.day", nil)
	raw := m.message("dev@example.com", "482913")

	mediaType, params, body := topLevel(t, raw)
	if mediaType != "multipart/alternative" {
		t.Fatalf("media type = %q, want multipart/alternative", mediaType)
	}

	parts := readParts(t, body, params["boundary"])
	assertPart(t, parts, "text/plain", "482913")
	assertPart(t, parts, "text/html", "482913")
	if _, ok := parts["image/png"]; ok {
		t.Error("unexpected image/png part without a logo")
	}
}

// TestSMTPMailerMessageWithLogo asserts a logo produces a multipart/related
// wrapping the alternative body plus an inline cid: image part, and that the
// HTML references that cid.
func TestSMTPMailerMessageWithLogo(t *testing.T) {
	m := NewSMTPMailer("smtp.example.com", "587", "user", "pass", "login@unbusy.day", []byte("\x89PNGfakebytes"))
	raw := m.message("dev@example.com", "482913")

	mediaType, params, body := topLevel(t, raw)
	if mediaType != "multipart/related" {
		t.Fatalf("media type = %q, want multipart/related", mediaType)
	}

	parts := readParts(t, body, params["boundary"])
	if _, ok := parts["image/png"]; !ok {
		t.Fatal("missing image/png part")
	}

	alt, ok := parts["multipart/alternative"]
	if !ok {
		t.Fatal("missing nested multipart/alternative")
	}
	if !strings.Contains(alt, "cid:"+logoCID) {
		t.Errorf("HTML does not reference cid:%s", logoCID)
	}
	if !strings.Contains(alt, "482913") {
		t.Error("nested body missing code")
	}
}

// topLevel parses the raw message and returns its top-level media type, params,
// and undecoded body.
func topLevel(t *testing.T, raw []byte) (string, map[string]string, string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content-type: %v", err)
	}
	b, err := io.ReadAll(msg.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return mt, params, string(b)
}

// readParts walks a multipart body and maps each part's media type to its raw
// (undecoded) content. For a nested multipart part it maps the whole raw part.
func readParts(t *testing.T, body, boundary string) map[string]string {
	t.Helper()
	out := map[string]string{}
	mr := multipart.NewReader(strings.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		mt, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		out[mt] = string(content)
	}
	return out
}

func assertPart(t *testing.T, parts map[string]string, mediaType, want string) {
	t.Helper()
	body, ok := parts[mediaType]
	if !ok {
		t.Errorf("missing %s part", mediaType)
		return
	}
	if !strings.Contains(body, want) {
		t.Errorf("%s part missing %q: %q", mediaType, want, body)
	}
}
