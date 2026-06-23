package frontend

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Suppressor is the webhook's view of the auth service; *auth.Service satisfies
// it. Bounce/complaint feedback lands an address on the suppression list.
type Suppressor interface {
	Suppress(ctx context.Context, email, reason, detail string) error
}

// snsMessage is the SNS HTTP-subscription envelope (the fields we need to
// verify the signature and dispatch). Message is itself a JSON string.
type snsMessage struct {
	Type             string `json:"Type"`
	MessageID        string `json:"MessageId"`
	Token            string `json:"Token"`
	TopicArn         string `json:"TopicArn"`
	Subject          string `json:"Subject"`
	Message          string `json:"Message"`
	SubscribeURL     string `json:"SubscribeURL"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
	Signature        string `json:"Signature"`
	SigningCertURL   string `json:"SigningCertURL"`
}

// sesNotification is the SES bounce/complaint payload carried in the SNS
// Message field. We read only the recipients we must suppress.
type sesNotification struct {
	NotificationType string `json:"notificationType"`
	Bounce           struct {
		BounceType        string `json:"bounceType"` // Permanent | Transient | Undetermined
		BouncedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint struct {
		ComplainedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
}

// SESWebhookHandler receives SES feedback over an SNS HTTP subscription. It
// verifies the SNS signature, auto-confirms the subscription, and suppresses
// permanently-bounced or complaining recipients. expectedTopicARN locks it to
// our topic; an empty value rejects everything (misconfiguration safety).
func SESWebhookHandler(s Suppressor, expectedTopicARN string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		var msg snsMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if expectedTopicARN == "" || msg.TopicArn != expectedTopicARN {
			log.Printf("ses webhook: rejecting topic %q", msg.TopicArn)
			http.Error(w, "unexpected topic", http.StatusForbidden)
			return
		}
		if err := verifySNSSignature(&msg); err != nil {
			log.Printf("ses webhook: signature: %v", err)
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}

		switch msg.Type {
		case "SubscriptionConfirmation":
			// Confirm by fetching the (signature-verified) SubscribeURL.
			if err := confirmSubscription(r.Context(), msg.SubscribeURL); err != nil {
				log.Printf("ses webhook: confirm: %v", err)
				http.Error(w, "confirm failed", http.StatusBadGateway)
				return
			}
			log.Printf("ses webhook: subscription confirmed for %s", msg.TopicArn)
		case "Notification":
			if err := handleNotification(r.Context(), s, msg.Message); err != nil {
				log.Printf("ses webhook: notification: %v", err)
				http.Error(w, "process failed", http.StatusInternalServerError)
				return
			}
		default:
			log.Printf("ses webhook: ignoring type %q", msg.Type)
		}
		w.WriteHeader(http.StatusOK)
	})
}

// handleNotification suppresses every permanently-bounced or complaining
// recipient. Transient bounces are ignored — they may deliver on retry.
func handleNotification(ctx context.Context, s Suppressor, message string) error {
	var n sesNotification
	if err := json.Unmarshal([]byte(message), &n); err != nil {
		return fmt.Errorf("parse ses message: %w", err)
	}
	switch n.NotificationType {
	case "Bounce":
		if n.Bounce.BounceType != "Permanent" {
			return nil
		}
		for _, rcpt := range n.Bounce.BouncedRecipients {
			if err := s.Suppress(ctx, rcpt.EmailAddress, "bounce", n.Bounce.BounceType); err != nil {
				return err
			}
			log.Printf("ses webhook: suppressed bounce %s", rcpt.EmailAddress)
		}
	case "Complaint":
		for _, rcpt := range n.Complaint.ComplainedRecipients {
			if err := s.Suppress(ctx, rcpt.EmailAddress, "complaint", ""); err != nil {
				return err
			}
			log.Printf("ses webhook: suppressed complaint %s", rcpt.EmailAddress)
		}
	}
	return nil
}

func confirmSubscription(ctx context.Context, subscribeURL string) error {
	if err := validateAWSHost(subscribeURL); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscribeURL, nil)
	if err != nil {
		return err
	}
	resp, err := snsHTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("subscribe url returned %d", resp.StatusCode)
	}
	return nil
}

// snsHTTP bounds the cert/confirm fetches; SNS endpoints are AWS-hosted HTTPS.
var snsHTTP = &http.Client{Timeout: 10 * time.Second}

// verifySNSSignature checks the message against its SNS signing certificate,
// proving it really came from SNS and wasn't tampered with.
func verifySNSSignature(m *snsMessage) error {
	if err := validateAWSHost(m.SigningCertURL); err != nil {
		return err
	}
	signed, err := canonicalSNS(m)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, m.SigningCertURL, nil)
	if err != nil {
		return err
	}
	resp, err := snsHTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	pemBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return err
	}
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return errors.New("signing cert: no PEM block")
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return fmt.Errorf("parse cert: %w", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("signing cert: not RSA")
	}

	// SignatureVersion 1 signs with SHA1, version 2 with SHA256.
	hashAlg, sum := crypto.SHA1, sha1.Sum(signed)
	if m.SignatureVersion == "2" {
		hashAlg = crypto.SHA256
		s256 := sha256.Sum256(signed)
		return rsa.VerifyPKCS1v15(pub, hashAlg, s256[:], sig)
	}
	return rsa.VerifyPKCS1v15(pub, hashAlg, sum[:], sig)
}

// canonicalSNS builds the exact signing string SNS uses: sorted key\nvalue\n
// pairs over a fixed field set that depends on the message type.
func canonicalSNS(m *snsMessage) ([]byte, error) {
	var keys []string
	switch m.Type {
	case "Notification":
		keys = []string{"Message", "MessageId", "Subject", "Timestamp", "TopicArn", "Type"}
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		keys = []string{"Message", "MessageId", "SubscribeURL", "Timestamp", "Token", "TopicArn", "Type"}
	default:
		return nil, fmt.Errorf("cannot sign type %q", m.Type)
	}
	vals := map[string]string{
		"Message": m.Message, "MessageId": m.MessageID, "Subject": m.Subject,
		"SubscribeURL": m.SubscribeURL, "Timestamp": m.Timestamp,
		"Token": m.Token, "TopicArn": m.TopicArn, "Type": m.Type,
	}
	var b strings.Builder
	for _, k := range keys {
		// Subject is optional: omit the pair entirely when absent.
		if k == "Subject" && m.Subject == "" {
			continue
		}
		b.WriteString(k)
		b.WriteByte('\n')
		b.WriteString(vals[k])
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// validateAWSHost ensures cert/confirm URLs point at amazonaws.com over HTTPS,
// so a forged message can't redirect us to an attacker-controlled cert.
func validateAWSHost(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("non-https url: %s", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host != "amazonaws.com" && !strings.HasSuffix(host, ".amazonaws.com") {
		return fmt.Errorf("non-aws host: %s", host)
	}
	return nil
}
