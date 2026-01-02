package imap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

var (
	ErrConnectionFailed  = errors.New("failed to connect to IMAP server")
	ErrTLSFailed         = errors.New("TLS handshake failed")
	ErrAuthFailed        = errors.New("authentication failed")
	ErrTimeout           = errors.New("connection timed out")
	ErrListFoldersFailed = errors.New("failed to list folders")
	ErrFolderNotFound    = errors.New("folder not found")
	ErrSelectFailed      = errors.New("failed to select folder")
	ErrMessageNotFound   = errors.New("message not found")
	ErrFetchFailed       = errors.New("failed to fetch message")
)

const defaultTimeout = 30 * time.Second

type ConnectionResult struct {
	Folders []string
}

// Folder represents an IMAP mailbox with metadata.
type Folder struct {
	Name         string `json:"name"`
	MessageCount uint32 `json:"message_count"`
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

func FetchRaw(ctx context.Context, host string, port int, user, password, folder string, uid uint32) ([]byte, error) {
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

	defer func() { _ = client.Logout().Wait() }()

	selectCmd := client.Select(folder, nil)

	if _, err := selectCmd.Wait(); err != nil {
		if isNoSuchMailboxError(err) {
			return nil, fmt.Errorf("%w: folder %s: %v", ErrFolderNotFound, folder, err)
		}

		return nil, fmt.Errorf("%w: folder %s: %v", ErrSelectFailed, folder, err)
	}

	fetchOptions := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	fetchCmd := client.Fetch(uidSet, fetchOptions)

	messages, err := fetchCmd.Collect()

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: UID %d in folder %s", ErrMessageNotFound, uid, folder)
	}

	rawBody := messages[0].FindBodySection(&imap.FetchItemBodySection{})

	if rawBody == nil {
		return nil, fmt.Errorf("%w: UID %d in folder %s (empty body)", ErrMessageNotFound, uid, folder)
	}

	return rawBody, nil
}

// FetchUIDsAfter returns UIDs greater than the given UID in the folder.
// If afterUID is 0, returns all UIDs in the folder.
func FetchUIDsAfter(ctx context.Context, host string, port int, user, password, folder string, afterUID uint32) ([]uint32, error) {
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

	defer func() { _ = client.Logout().Wait() }()

	selectData, err := client.Select(folder, nil).Wait()

	if err != nil {
		if isNoSuchMailboxError(err) {
			return nil, fmt.Errorf("%w: folder %s: %v", ErrFolderNotFound, folder, err)
		}

		return nil, fmt.Errorf("%w: folder %s: %v", ErrSelectFailed, folder, err)
	}

	// If folder is empty, return empty slice
	if selectData.NumMessages == 0 {
		return []uint32{}, nil
	}

	// Search for UIDs > afterUID
	var searchCriteria *imap.SearchCriteria

	if afterUID > 0 {
		// UID range from afterUID+1 to * (0 means unbounded/max)
		uidRange := imap.UIDRange{Start: imap.UID(afterUID + 1), Stop: 0}
		searchCriteria = &imap.SearchCriteria{
			UID: []imap.UIDSet{{uidRange}},
		}
	} else {
		searchCriteria = &imap.SearchCriteria{} // All messages
	}

	searchData, err := client.UIDSearch(searchCriteria, nil).Wait()

	if err != nil {
		return nil, fmt.Errorf("searching UIDs: %w", err)
	}

	uids := make([]uint32, 0, len(searchData.AllUIDs()))

	for _, uid := range searchData.AllUIDs() {
		uids = append(uids, uint32(uid))
	}

	return uids, nil
}

// ListFolders returns all folders with message counts.
func ListFolders(ctx context.Context, host string, port int, user, password string) ([]Folder, error) {
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

	defer func() { _ = client.Logout().Wait() }()

	mailboxes, err := client.List("", "*", nil).Collect()

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrListFoldersFailed, err)
	}

	folders := make([]Folder, 0, len(mailboxes))

	for _, mbox := range mailboxes {
		// Get message count via STATUS command
		statusOpts := &imap.StatusOptions{NumMessages: true}
		statusData, err := client.Status(mbox.Mailbox, statusOpts).Wait()

		if err != nil {
			slog.Debug("STATUS failed for folder", "folder", mbox.Mailbox, "error", err)
			folders = append(folders, Folder{
				Name:         mbox.Mailbox,
				MessageCount: 0,
			})

			continue
		}

		var count uint32

		if statusData.NumMessages != nil {
			count = *statusData.NumMessages
		}

		folders = append(folders, Folder{
			Name:         mbox.Mailbox,
			MessageCount: count,
		})
	}

	return folders, nil
}

// Dial connects to an IMAP server with TLS (implicit on port 993, STARTTLS otherwise).
// The opts parameter can be nil for simple connections.
func Dial(ctx context.Context, host string, port int, opts *imapclient.Options) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return nil, err
	}

	if port == 993 {
		return dialImplicitTLS(ctx, conn, host, opts)
	}

	return dialStartTLS(conn, host, opts)
}

func dial(ctx context.Context, addr, host string, port int) (*imapclient.Client, error) {
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", addr)

	if err != nil {
		return nil, err
	}

	if port == 993 {
		return dialImplicitTLS(ctx, conn, host, nil)
	}

	return dialStartTLS(conn, host, nil)
}

func dialImplicitTLS(ctx context.Context, conn net.Conn, host string, opts *imapclient.Options) (*imapclient.Client, error) {
	tlsConfig := &tls.Config{ServerName: host}
	tlsConn := tls.Client(conn, tlsConfig)

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		conn.Close()

		return nil, err
	}

	if opts == nil {
		opts = &imapclient.Options{}
	}

	opts.TLSConfig = tlsConfig

	return imapclient.New(tlsConn, opts), nil
}

func dialStartTLS(conn net.Conn, host string, opts *imapclient.Options) (*imapclient.Client, error) {
	if opts == nil {
		opts = &imapclient.Options{}
	}

	opts.TLSConfig = &tls.Config{ServerName: host}

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

func isNoSuchMailboxError(err error) bool {
	var imapErr *imap.Error

	if errors.As(err, &imapErr) {
		return imapErr.Code == imap.ResponseCodeNonExistent ||
			imapErr.Code == imap.ResponseCodeTryCreate
	}

	return false
}
