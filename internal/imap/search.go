package imap

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

type EmailSummary struct {
	UID     uint32
	Date    time.Time
	From    string
	Subject string
	Flags   []imap.Flag
}

// Search searches for emails in a mailbox matching the given criteria.
func Search(c *imapclient.Client, mailbox string, query, from, since, before string, unseenOnly bool, limit int) ([]EmailSummary, error) {
	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, fmt.Errorf("select %s: %w", mailbox, err)
	}

	criteria := &imap.SearchCriteria{}

	if from != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{
			Key:   "FROM",
			Value: from,
		})
	}

	if query != "" {
		criteria.Header = append(criteria.Header, imap.SearchCriteriaHeaderField{
			Key:   "SUBJECT",
			Value: query,
		})
	}

	if since != "" {
		t, err := time.Parse("2006-01-02", since)
		if err == nil {
			criteria.Since = t
		}
	} else {
		// Default: last 7 days to avoid fetching the entire mailbox
		criteria.Since = time.Now().AddDate(0, 0, -7)
	}

	if before != "" {
		t, err := time.Parse("2006-01-02", before)
		if err == nil {
			criteria.Before = t
		}
	}

	if unseenOnly {
		criteria.NotFlag = append(criteria.NotFlag, imap.FlagSeen)
	}

	searchData, err := c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("IMAP search: %w", err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}

	// Reverse to get newest first
	for i, j := 0, len(uids)-1; i < j; i, j = i+1, j-1 {
		uids[i], uids[j] = uids[j], uids[i]
	}

	// Limit results
	if limit > 0 && len(uids) > limit {
		uids = uids[:limit]
	}

	uidSet := imap.UIDSet{}
	for _, uid := range uids {
		uidSet.AddNum(uid)
	}

	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		Flags:    true,
		UID:      true,
	}

	messages, err := c.Fetch(uidSet, fetchOptions).Collect()
	if err != nil {
		return nil, fmt.Errorf("IMAP fetch: %w", err)
	}

	summaries := make([]EmailSummary, 0, len(messages))
	for _, msg := range messages {
		if msg.Envelope == nil {
			continue
		}
		fromStr := ""
		if len(msg.Envelope.From) > 0 {
			a := msg.Envelope.From[0]
			if a.Name != "" {
				fromStr = fmt.Sprintf("%s <%s@%s>", a.Name, a.Mailbox, a.Host)
			} else {
				fromStr = fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
			}
		}
		summaries = append(summaries, EmailSummary{
			UID:     uint32(msg.UID),
			Date:    msg.Envelope.Date,
			From:    fromStr,
			Subject: msg.Envelope.Subject,
			Flags:   msg.Flags,
		})
	}

	// Sort by UID descending (newest first)
	// Messages from FETCH may not be in order
	sortByUIDDesc(summaries)

	return summaries, nil
}

func sortByUIDDesc(s []EmailSummary) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].UID > s[j-1].UID; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func FormatSummaries(accountID, mailbox string, summaries []EmailSummary) string {
	if len(summaries) == 0 {
		return fmt.Sprintf("No emails found in %s/%s.", accountID, mailbox)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d email(s) in %s/%s:\n\n", len(summaries), accountID, mailbox)
	for i, s := range summaries {
		date := s.Date.Format("2006-01-02")
		fmt.Fprintf(&sb, "%d. [UID:%d] %s | From: %s | Subject: %s\n", i+1, s.UID, date, s.From, s.Subject)
	}
	return sb.String()
}
