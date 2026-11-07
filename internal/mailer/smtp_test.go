package mailer

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestPasswordResetURL(t *testing.T) {
	got, err := PasswordResetURL("https://accounts.example/reset?source=email", "af_pr_secret")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://accounts.example/reset?source=email&token=af_pr_secret" {
		t.Fatalf("unexpected reset URL: %s", got)
	}
}

func TestPasswordResetURLRejectsInsecurePublicURL(t *testing.T) {
	if _, err := PasswordResetURL("http://accounts.example/reset", "token"); err == nil {
		t.Fatal("expected public HTTP URL to be rejected")
	}
}

func TestEmailVerificationURL(t *testing.T) {
	got, err := EmailVerificationURL("https://accounts.example/verify", "env-123", "af_ev_secret")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://accounts.example/verify?environment_id=env-123&token=af_ev_secret" {
		t.Fatalf("unexpected verification URL: %s", got)
	}
}

func TestNewSMTPRequiresTLSForCredentials(t *testing.T) {
	if _, err := NewSMTP("smtp.example:587", "AuthForge <no-reply@example.com>", "user", "password", false); err == nil {
		t.Fatal("expected SMTP credentials without STARTTLS to be rejected")
	}
}

func TestSendPasswordReset(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	message := make(chan string, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				serverErr <- err
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO "):
				_, _ = fmt.Fprint(conn, "250-localhost\r\n250 SIZE 1000000\r\n")
			case strings.HasPrefix(line, "MAIL FROM:"):
				_, _ = fmt.Fprint(conn, "250 sender ok\r\n")
			case strings.HasPrefix(line, "RCPT TO:"):
				_, _ = fmt.Fprint(conn, "250 recipient ok\r\n")
			case strings.TrimSpace(line) == "DATA":
				_, _ = fmt.Fprint(conn, "354 send data\r\n")
				var body strings.Builder
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						serverErr <- err
						return
					}
					if line == ".\r\n" {
						break
					}
					body.WriteString(line)
				}
				message <- body.String()
				_, _ = fmt.Fprint(conn, "250 queued\r\n")
			case strings.TrimSpace(line) == "QUIT":
				_, _ = fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				_, _ = fmt.Fprint(conn, "500 unexpected command\r\n")
			}
		}
	}()

	sender, err := NewSMTP(listener.Addr().String(), "AuthForge <no-reply@example.com>", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.SendPasswordReset(context.Background(), "ada@example.com", "https://accounts.example/reset?token=secret"); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-message:
		if !strings.Contains(got, "To: <ada@example.com>") || !strings.Contains(got, "https://accounts.example/reset?token=secret") {
			t.Fatalf("reset message missing recipient or link: %s", got)
		}
	case err := <-serverErr:
		t.Fatal(err)
	}
}
