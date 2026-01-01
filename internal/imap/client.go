package imap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

var (
	ErrConnectionFailed  = errors.New("failed to connect to IMAP server")
	ErrTLSFailed         = errors.New("TLS handshake failed")
	ErrAuthFailed        = errors.New("authentication failed")
	ErrTimeout           = errors.New("connection timed out")
	ErrListFoldersFailed = errors.New("failed to list folders")
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

	client, err := dial(ctx, addr, host, port)

	if err != nil {
		return nil, classifyError(err)
	}

	defer client.Close()

	if err := client.Login(user, password).Wait(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	mailboxes, err := client.List("", "*", nil).Collect()

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListFoldersFailed, err)
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

func dial(ctx context.Context, addr, host string, port int) (*imapclient.Client, error) {
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return nil, err
	}

	if port == 993 {
		return dialImplicitTLS(ctx, conn, host)
	}

	return dialStartTLS(conn, host)
}

func dialImplicitTLS(ctx context.Context, conn net.Conn, host string) (*imapclient.Client, error) {
	tlsConfig := &tls.Config{ServerName: host}
	tlsConn := tls.Client(conn, tlsConfig)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()

		return nil, err
	}

	return imapclient.New(tlsConn, nil), nil
}

func dialStartTLS(conn net.Conn, host string) (*imapclient.Client, error) {
	opts := &imapclient.Options{
		TLSConfig: &tls.Config{ServerName: host},
	}

	client, err := imapclient.NewStartTLS(conn, opts)

	if err != nil {
		conn.Close()

		return nil, err
	}

	return client, nil
}

func classifyError(err error) error {

	if isTimeoutError(err) {
		return fmt.Errorf("%w: %v", ErrTimeout, err)
	}

	if isTLSError(err) {
		return fmt.Errorf("%w: %v", ErrTLSFailed, err)
	}

	return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
}

func isTimeoutError(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}

func isTLSError(err error) bool {
	var recordErr *tls.RecordHeaderError
	var certVerifyErr *tls.CertificateVerificationError
	var unknownAuthErr x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var certInvalidErr x509.CertificateInvalidError

	return errors.As(err, &recordErr) ||
		errors.As(err, &certVerifyErr) ||
		errors.As(err, &unknownAuthErr) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &certInvalidErr)
}
