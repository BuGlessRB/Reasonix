package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/platform/feedback"
)

const feedbackCommandTimeout = 90 * time.Second

const feedbackUsage = `usage:
  /feedback <bug|idea|question|other> [--yes] <text>   (--yes only directly after the category)
  /feedback list                                       show your feedback and its status
  /feedback name <nickname>                            set the nickname reports go out under`

const feedbackPublicNotice = "Your text and nickname become a PUBLIC GitHub issue in esengine/DeepSeek-Reasonix. Do not include secrets or private code. Security problems belong in SECURITY.md, not here."

func feedbackArgItems(prior []string) []SlashItem {
	if len(prior) > 1 {
		return nil
	}
	return []SlashItem{
		{Label: "bug", Insert: "bug", Hint: "report something broken"},
		{Label: "idea", Insert: "idea", Hint: "suggest an improvement"},
		{Label: "question", Insert: "question", Hint: "ask about usage"},
		{Label: "other", Insert: "other", Hint: "anything else"},
		{Label: "list", Insert: "list", Hint: "show your feedback and its status"},
		{Label: "name", Insert: "name", Hint: "set the nickname reports go out under"},
	}
}

// feedbackCommand answers a /feedback line as notices. The network part runs
// off the calling goroutine so a slow service never holds the input path.
func (c *Controller) feedbackCommand(args string) {
	if c.feedback.Service == nil || !c.feedback.Surface.Valid() {
		c.notice("feedback is not available in this session")
		return
	}
	verb, rest := args, ""
	if i := strings.IndexAny(args, " \t\r\n"); i >= 0 {
		verb, rest = args[:i], strings.TrimSpace(args[i:])
	}
	switch strings.ToLower(verb) {
	case "", "help":
		c.notice(feedbackUsage)
	case "list", "ls":
		go c.feedbackList()
	case "name":
		if err := c.SetFeedbackDisplayName(rest); err != nil {
			c.notice(feedbackFailure(err))
			return
		}
		c.notice("nickname set to " + c.FeedbackDisplayName())
	default:
		c.feedbackSend(feedback.Category(strings.ToLower(verb)), rest)
	}
}

func (c *Controller) feedbackSend(category feedback.Category, text string) {
	confirmed := false
	body := text
	flag, after := text, ""
	if i := strings.IndexAny(text, " \t\r\n"); i >= 0 {
		flag, after = text[:i], text[i:]
	}
	if flag == "--yes" {
		confirmed, body = true, strings.TrimSpace(after)
	}
	name := c.FeedbackDisplayName()
	switch {
	case body == "":
		c.notice(feedbackUsage)
	case name == "":
		c.notice("set a nickname first (it is shown publicly): /feedback name <nickname>")
	case !confirmed:
		c.notice(feedbackPublicNotice + "\nNickname: " + name + "\nNothing was sent. To send it, run the same line again with --yes.")
	default:
		c.notice("sending feedback...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
			defer cancel()
			got, err := c.SubmitFeedback(ctx, feedback.Draft{
				Category: category, Body: body, DisplayName: name,
				Env: feedback.EnvContext{Surface: c.feedback.Surface},
			})
			if err != nil {
				c.notice(feedbackFailure(err))
				return
			}
			msg := "Sent. Receipt " + got.Receipt + " - follow it with /feedback list."
			if got.Redacted {
				msg += "\nSecret-looking text was masked before sending."
			}
			c.notice(msg)
		}()
	}
}

func (c *Controller) feedbackList() {
	ctx, cancel := context.WithTimeout(context.Background(), feedbackCommandTimeout)
	defer cancel()
	got, err := c.ListFeedback(ctx)
	if err != nil {
		c.notice(feedbackFailure(err))
		return
	}
	if len(got.Items) == 0 {
		c.notice("no feedback sent from this machine yet")
		return
	}
	var b strings.Builder
	if got.Offline {
		b.WriteString("(offline - showing what this machine remembers; statuses may be out of date)\n")
	}
	for _, it := range got.Items {
		fmt.Fprintf(&b, "%s  %-8s %-11s %s%s\n", it.Receipt, it.Category, feedbackStatusText(it), feedbackIssueRef(it), it.TitleSnippet)
	}
	c.notice(strings.TrimRight(b.String(), "\n"))
}

func feedbackIssueRef(it feedback.Item) string {
	if it.IssueNumber == nil {
		return ""
	}
	return fmt.Sprintf("#%d ", *it.IssueNumber)
}

func feedbackStatusText(it feedback.Item) string {
	switch it.Status {
	case feedback.StatusFixed:
		if it.ResolvedVersion == "next" || it.ResolvedVersion == "" {
			return "fixed (next version)"
		}
		return "fixed in " + it.ResolvedVersion
	case feedback.StatusDuplicate:
		if it.DuplicateOf != nil {
			return fmt.Sprintf("duplicate of #%d", *it.DuplicateOf)
		}
		return "duplicate"
	case feedback.StatusWontFix:
		return "won't fix"
	case feedback.StatusInProgress:
		return "in progress"
	}
	return string(it.Status)
}

func feedbackFailure(err error) string {
	var invalid *feedback.InvalidError
	switch {
	case errors.As(err, &invalid):
		return "feedback not sent: " + invalid.Field + " is not acceptable (" + invalid.Reason + ")"
	case errors.Is(err, feedback.ErrTooLarge):
		return "feedback not sent: it is too large - shorten the text"
	case errors.Is(err, feedback.ErrRateLimited):
		if after := feedback.RetryAfter(err); after > 0 {
			return "feedback not sent: too many submissions, try again in " + after.Round(time.Second).String()
		}
		return "feedback not sent: too many submissions, try again later"
	case errors.Is(err, feedback.ErrDisabled):
		return "feedback is switched off right now; open an issue on GitHub instead"
	case errors.Is(err, feedback.ErrBusy):
		return "feedback is not being accepted right now (the service is at its daily capacity) - try again tomorrow"
	case errors.Is(err, feedback.ErrImageMetadata):
		return "feedback not sent: an image could not be cleaned of its metadata"
	case errors.Is(err, feedback.ErrDuplicate):
		return "the same feedback was just sent"
	case errors.Is(err, feedback.ErrBadToken):
		return "the feedback service did not accept this install's identity - run the command again"
	case errors.Is(err, feedback.ErrOffline):
		return "cannot reach the feedback service - check the network and run the command again"
	case errors.Is(err, feedback.ErrUnavailable):
		return "the feedback service failed - try again later"
	}
	return "feedback: " + err.Error()
}
