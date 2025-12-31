package imap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

var (
	ErrConnectionFailed = errors.New("failed to connect to IMAP server")
	ErrTLSFailed        = errors.New("TLS handshake failed")
	ErrAuthFailed       = errors.New("authentication failed")
	ErrTimeout          = errors.New("connection timed out")
)

const defaultTimeout = 30 * time.Second

type ConnectionResult struct {
	Folders []string
}

func TestConnection(ctx context.Context, host string, port int, user, password string) (*ConnectionResult, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	opts := &imapclient.Options{
		TLSConfig: &tls.Config{
			ServerName: host,
		},
	}

	var client *imapclient.Client
	var err error

	connCh := make(chan struct {
		client *imapclient.Client
		err    error
	}, 1)

	go func() {
		var c *imapclient.Client
		var e error

		if port == 993 {
			c, e = imapclient.DialTLS(addr, opts)
		} else {
			c, e = imapclient.DialStartTLS(addr, opts)
		}

		connCh <- struct {
			client *imapclient.Client
			err    error
		}{c, e}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrTimeout, ctx.Err())
	case result := <-connCh:
		client = result.client
		err = result.err
	}

	if err != nil {
		if isTimeoutError(err) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}

		if isTLSError(err) {
			return nil, fmt.Errorf("%w: %v", ErrTLSFailed, err)
		}

		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	defer client.Close()

	if err := client.Login(user, password).Wait(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	mailboxes, err := client.List("", "*", nil).Collect()

	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}

	folders := make([]string, 0, len(mailboxes))

	for _, mbox := range mailboxes {
		folders = append(folders, mbox.Mailbox)
	}

	_ = client.Logout().Wait()

	return &ConnectionResult{
		Folders: folders,
	}, nil
}

func isTimeoutError(err error) bool {

	if err == nil {
		return false
	}

	var netErr net.Error

	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}

func isTLSError(err error) bool {

	if err == nil {
		return false
	}

	var tlsErr *tls.RecordHeaderError

	if errors.As(err, &tlsErr) {
		return true
	}

	// Check for certificate errors
	var certErr *tls.CertificateVerificationError

	if errors.As(err, &certErr) {
		return true
	}

	return false
}
