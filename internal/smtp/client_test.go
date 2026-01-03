package smtp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	gosmtp "net/smtp"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockSMTPServer is a simple SMTP server for testing.
type mockSMTPServer struct {
	listener net.Listener
	authUser string
	authPass string
	failAuth bool
	failSend bool

	mu            sync.Mutex
	receivedMail  []receivedEmail
	authenticated bool
}

type receivedEmail struct {
	from    string
	to      []string
	message []byte
}

func newMockSMTPServer(t *testing.T, port int) *mockSMTPServer {
	t.Helper()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("failed to start mock SMTP server: %v", err)
	}

	server := &mockSMTPServer{
		listener: listener,
		authUser: "testuser",
		authPass: "testpass",
	}

	go server.serve()

	return server
}

func (s *mockSMTPServer) addr() string {
	return s.listener.Addr().String()
}

func (s *mockSMTPServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

func (s *mockSMTPServer) close() {
	s.listener.Close()
}

func (s *mockSMTPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}

		go s.handleConn(conn)
	}
}

func (s *mockSMTPServer) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Send greeting
	s.writeLine(writer, "220 localhost ESMTP Mock")

	var from string
	var to []string
	var inData bool
	var dataLines []string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)

		if inData {
			if line == "." {
				inData = false

				s.mu.Lock()
				s.receivedMail = append(s.receivedMail, receivedEmail{
					from:    from,
					to:      to,
					message: []byte(strings.Join(dataLines, "\r\n")),
				})
				s.mu.Unlock()

				if s.failSend {
					s.writeLine(writer, "550 Delivery failed")
				} else {
					s.writeLine(writer, "250 OK")
				}

				from = ""
				to = nil
				dataLines = nil
			} else {
				dataLines = append(dataLines, line)
			}

			continue
		}

		cmd := strings.ToUpper(line)
		if len(cmd) > 4 {
			cmd = cmd[:4]
		}

		switch cmd {
		case "EHLO", "HELO":
			s.writeLine(writer, "250-localhost")
			s.writeLine(writer, "250-AUTH PLAIN LOGIN")
			s.writeLine(writer, "250 OK")

		case "AUTH":
			if s.failAuth {
				s.writeLine(writer, "535 Authentication failed")
				continue
			}

			parts := strings.Fields(line)
			if len(parts) < 2 {
				s.writeLine(writer, "501 Syntax error")

				continue
			}

			mechanism := strings.ToUpper(parts[1])

			switch mechanism {
			case "PLAIN":
				if len(parts) == 3 {
					// Inline credentials
					s.mu.Lock()
					s.authenticated = true
					s.mu.Unlock()
					s.writeLine(writer, "235 Authentication successful")
				} else {
					s.writeLine(writer, "334 ")
					reader.ReadString('\n') // Read credentials
					s.mu.Lock()
					s.authenticated = true
					s.mu.Unlock()
					s.writeLine(writer, "235 Authentication successful")
				}

			case "LOGIN":
				s.writeLine(writer, "334 Username:")
				reader.ReadString('\n') // Read username
				s.writeLine(writer, "334 Password:")
				reader.ReadString('\n') // Read password
				s.mu.Lock()
				s.authenticated = true
				s.mu.Unlock()
				s.writeLine(writer, "235 Authentication successful")

			default:
				s.writeLine(writer, "504 Unrecognized authentication type")
			}

		case "MAIL":
			// Parse MAIL FROM:<address>
			if idx := strings.Index(line, "<"); idx != -1 {
				if end := strings.Index(line, ">"); end != -1 {
					from = line[idx+1 : end]
				}
			}

			s.writeLine(writer, "250 OK")

		case "RCPT":
			// Parse RCPT TO:<address>
			if idx := strings.Index(line, "<"); idx != -1 {
				if end := strings.Index(line, ">"); end != -1 {
					to = append(to, line[idx+1:end])
				}
			}

			s.writeLine(writer, "250 OK")

		case "DATA":
			s.writeLine(writer, "354 Start mail input")
			inData = true

		case "QUIT":
			s.writeLine(writer, "221 Bye")

			return

		default:
			s.writeLine(writer, "502 Command not implemented")
		}
	}
}

func (s *mockSMTPServer) writeLine(w *bufio.Writer, line string) {
	w.WriteString(line + "\r\n")
	w.Flush()
}

func TestTestConnection_Success(t *testing.T) {
	server := newMockSMTPServer(t, 0)
	defer server.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := TestConnection(ctx, "127.0.0.1", server.port(), "testuser", "testpass")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestTestConnection_AuthFailed(t *testing.T) {
	server := newMockSMTPServer(t, 0)
	server.failAuth = true
	defer server.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := TestConnection(ctx, "127.0.0.1", server.port(), "testuser", "wrongpass")
	if err == nil {
		t.Error("expected error for wrong password")
	}

	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

func TestTestConnection_ConnectionFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to connect to a port with nothing listening
	err := TestConnection(ctx, "127.0.0.1", 59999, "testuser", "testpass")
	if err == nil {
		t.Error("expected connection error")
	}

	if !errors.Is(err, ErrConnectionFailed) {
		t.Errorf("expected ErrConnectionFailed, got %v", err)
	}
}

func TestTestConnection_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// This should timeout immediately
	err := TestConnection(ctx, "10.255.255.1", 587, "testuser", "testpass")
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestSendEmail_Success(t *testing.T) {
	server := newMockSMTPServer(t, 0)
	defer server.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		Host:     "127.0.0.1",
		Port:     server.port(),
		Username: "testuser",
		Password: "testpass",
	}

	message := []byte("From: sender@test.com\r\nTo: recipient@test.com\r\nSubject: Test\r\n\r\nHello!")

	err := SendEmail(ctx, cfg, "sender@test.com", []string{"recipient@test.com"}, message)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify the email was received
	server.mu.Lock()
	defer server.mu.Unlock()

	if len(server.receivedMail) != 1 {
		t.Fatalf("expected 1 email, got %d", len(server.receivedMail))
	}

	mail := server.receivedMail[0]

	if mail.from != "sender@test.com" {
		t.Errorf("expected from sender@test.com, got %s", mail.from)
	}

	if len(mail.to) != 1 || mail.to[0] != "recipient@test.com" {
		t.Errorf("expected to [recipient@test.com], got %v", mail.to)
	}
}

func TestSendEmail_MultipleRecipients(t *testing.T) {
	server := newMockSMTPServer(t, 0)
	defer server.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		Host:     "127.0.0.1",
		Port:     server.port(),
		Username: "testuser",
		Password: "testpass",
	}

	message := []byte("From: sender@test.com\r\nTo: a@test.com, b@test.com\r\nSubject: Test\r\n\r\nHello!")
	recipients := []string{"a@test.com", "b@test.com", "c@test.com"}

	err := SendEmail(ctx, cfg, "sender@test.com", recipients, message)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if len(server.receivedMail) != 1 {
		t.Fatalf("expected 1 email, got %d", len(server.receivedMail))
	}

	mail := server.receivedMail[0]

	if len(mail.to) != 3 {
		t.Errorf("expected 3 recipients, got %d", len(mail.to))
	}
}

func TestSendEmail_AuthFailed(t *testing.T) {
	server := newMockSMTPServer(t, 0)
	server.failAuth = true
	defer server.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		Host:     "127.0.0.1",
		Port:     server.port(),
		Username: "testuser",
		Password: "wrongpass",
	}

	message := []byte("From: sender@test.com\r\nTo: recipient@test.com\r\nSubject: Test\r\n\r\nHello!")

	err := SendEmail(ctx, cfg, "sender@test.com", []string{"recipient@test.com"}, message)
	if err == nil {
		t.Error("expected auth error")
	}

	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("expected ErrAuthFailed, got %v", err)
	}
}

func TestLoginAuth(t *testing.T) {
	auth := &loginAuth{username: "user", password: "pass"}

	// Test Start
	mech, resp, err := auth.Start(&gosmtp.ServerInfo{})
	if err != nil {
		t.Errorf("Start returned error: %v", err)
	}

	if mech != "LOGIN" {
		t.Errorf("expected mechanism LOGIN, got %s", mech)
	}

	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}

	// Test Next with username prompt
	resp, err = auth.Next([]byte("Username:"), true)
	if err != nil {
		t.Errorf("Next returned error: %v", err)
	}

	if string(resp) != "user" {
		t.Errorf("expected username 'user', got %s", resp)
	}

	// Test Next with password prompt
	resp, err = auth.Next([]byte("Password:"), true)
	if err != nil {
		t.Errorf("Next returned error: %v", err)
	}

	if string(resp) != "pass" {
		t.Errorf("expected password 'pass', got %s", resp)
	}

	// Test Next with more=false
	resp, err = auth.Next([]byte("done"), false)
	if err != nil {
		t.Errorf("Next returned error: %v", err)
	}

	if resp != nil {
		t.Errorf("expected nil response when more=false, got %v", resp)
	}
}

func TestLoginAuth_UnexpectedPrompt(t *testing.T) {
	auth := &loginAuth{username: "user", password: "pass"}

	_, err := auth.Next([]byte("Unexpected:"), true)
	if err == nil {
		t.Error("expected error for unexpected prompt")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected error
	}{
		{
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: ErrConnectionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			if !errors.Is(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
