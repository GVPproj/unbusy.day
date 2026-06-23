package frontend

import (
	"context"
	"testing"
)

type fakeSuppressor struct {
	calls [][2]string // {email, reason}
}

func (f *fakeSuppressor) Suppress(_ context.Context, email, reason, _ string) error {
	f.calls = append(f.calls, [2]string{email, reason})
	return nil
}

func TestHandleNotification(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    [][2]string
	}{
		{
			name:    "permanent bounce suppresses",
			message: `{"notificationType":"Bounce","bounce":{"bounceType":"Permanent","bouncedRecipients":[{"emailAddress":"gone@x.test"}]}}`,
			want:    [][2]string{{"gone@x.test", "bounce"}},
		},
		{
			name:    "transient bounce ignored",
			message: `{"notificationType":"Bounce","bounce":{"bounceType":"Transient","bouncedRecipients":[{"emailAddress":"busy@x.test"}]}}`,
			want:    nil,
		},
		{
			name:    "complaint suppresses",
			message: `{"notificationType":"Complaint","complaint":{"complainedRecipients":[{"emailAddress":"mad@x.test"}]}}`,
			want:    [][2]string{{"mad@x.test", "complaint"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeSuppressor{}
			if err := handleNotification(context.Background(), f, tc.message); err != nil {
				t.Fatalf("handleNotification: %v", err)
			}
			if len(f.calls) != len(tc.want) {
				t.Fatalf("calls = %v; want %v", f.calls, tc.want)
			}
			for i, w := range tc.want {
				if f.calls[i] != w {
					t.Fatalf("call %d = %v; want %v", i, f.calls[i], w)
				}
			}
		})
	}
}

func TestValidateAWSHost(t *testing.T) {
	ok := []string{
		"https://sns.us-west-2.amazonaws.com/cert.pem",
		"https://amazonaws.com/x",
	}
	bad := []string{
		"http://sns.us-west-2.amazonaws.com/cert.pem", // not https
		"https://evil.com/cert.pem",                   // wrong host
		"https://sns.amazonaws.com.evil.com/cert.pem", // suffix spoof
	}
	for _, u := range ok {
		if err := validateAWSHost(u); err != nil {
			t.Errorf("validateAWSHost(%q) = %v; want nil", u, err)
		}
	}
	for _, u := range bad {
		if err := validateAWSHost(u); err == nil {
			t.Errorf("validateAWSHost(%q) = nil; want error", u)
		}
	}
}
