package tropmail

import (
	"errors"
	"net/url"
)

// ErrMailboxID is returned when a mailbox UUID is required and missing.
var ErrMailboxID = errors.New("tropmail: mailbox ID is required")

func requireMailboxID(id string) (string, error) {
	if id == "" {
		return "", ErrMailboxID
	}
	return id, nil
}

func mailboxPath(mailboxID string, parts ...string) (string, error) {
	id, err := requireMailboxID(mailboxID)
	if err != nil {
		return "", err
	}
	path := "/mailboxes/" + url.PathEscape(id)
	for _, part := range parts {
		path += "/" + part
	}
	return path, nil
}
