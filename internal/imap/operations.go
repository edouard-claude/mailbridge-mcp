package imap

import (
	"fmt"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// MoveEmail moves an email from one mailbox to another.
func MoveEmail(c *imapclient.Client, fromMailbox string, uid uint32, toMailbox string) error {
	if _, err := c.Select(fromMailbox, nil).Wait(); err != nil {
		return fmt.Errorf("select %s: %w", fromMailbox, err)
	}

	uidSet := goimap.UIDSet{}
	uidSet.AddNum(goimap.UID(uid))

	if _, err := c.Move(uidSet, toMailbox).Wait(); err != nil {
		return fmt.Errorf("move UID %d from %s to %s: %w", uid, fromMailbox, toMailbox, err)
	}

	return nil
}

// MarkEmail changes flags on an email.
func MarkEmail(c *imapclient.Client, mailbox string, uid uint32, action string) error {
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("select %s: %w", mailbox, err)
	}

	uidSet := goimap.UIDSet{}
	uidSet.AddNum(goimap.UID(uid))

	var storeFlags *goimap.StoreFlags
	switch action {
	case "read":
		storeFlags = &goimap.StoreFlags{
			Op:     goimap.StoreFlagsAdd,
			Flags:  []goimap.Flag{goimap.FlagSeen},
			Silent: true,
		}
	case "unread":
		storeFlags = &goimap.StoreFlags{
			Op:     goimap.StoreFlagsDel,
			Flags:  []goimap.Flag{goimap.FlagSeen},
			Silent: true,
		}
	case "flag":
		storeFlags = &goimap.StoreFlags{
			Op:     goimap.StoreFlagsAdd,
			Flags:  []goimap.Flag{goimap.FlagFlagged},
			Silent: true,
		}
	case "unflag":
		storeFlags = &goimap.StoreFlags{
			Op:     goimap.StoreFlagsDel,
			Flags:  []goimap.Flag{goimap.FlagFlagged},
			Silent: true,
		}
	default:
		return fmt.Errorf("unknown action: %q (expected read, unread, flag, unflag)", action)
	}

	if err := c.Store(uidSet, storeFlags, nil).Close(); err != nil {
		return fmt.Errorf("store flags UID %d: %w", uid, err)
	}

	return nil
}

// ListMailboxes returns the list of mailbox names for the account.
func ListMailboxes(c *imapclient.Client) ([]string, error) {
	mailboxes, err := c.List("", "*", nil).Collect()
	if err != nil {
		return nil, fmt.Errorf("IMAP list: %w", err)
	}

	names := make([]string, 0, len(mailboxes))
	for _, mb := range mailboxes {
		names = append(names, mb.Mailbox)
	}
	return names, nil
}
