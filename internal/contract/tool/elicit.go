package tool

import "context"

// ElicitField is one value an external party asks the person to supply. With
// Choices it is picked from them (several when Multi); without, it is typed.
type ElicitField struct {
	Name        string
	Title       string
	Description string
	Choices     []string
	Multi       bool
	Default     []string // prefilled: the party's default, or what was given last time
}

// ElicitRequest is a form an external party — an MCP server mid tool call —
// asks the person at the host to fill. Source names that party on screen; Note
// is the host's own remark, such as why the last answer was not accepted.
type ElicitRequest struct {
	Source  string
	Message string
	Note    string
	Fields  []ElicitField
}

// ElicitReply is what the person gave back. Declined means they refused the
// form as a whole; Values holds the selections or text per field name.
type ElicitReply struct {
	Declined bool
	Values   map[string][]string
}

// Elicitor puts an external party's form in front of the person. A refusal is a
// reply, never an error: it goes back to the party, and the turn goes on. A nil
// elicitor (headless runs, sub-agents with no interactive parent) means nobody
// can be asked.
type Elicitor interface {
	Elicit(ctx context.Context, req ElicitRequest) (ElicitReply, error)
}

type elicitorContextKey struct{}

// WithElicitor stamps an elicitor onto a tool execution context.
func WithElicitor(ctx context.Context, e Elicitor) context.Context {
	if e == nil {
		return ctx
	}
	return context.WithValue(ctx, elicitorContextKey{}, e)
}

// ElicitorFrom returns the elicitor carried by ctx.
func ElicitorFrom(ctx context.Context) (Elicitor, bool) {
	if ctx == nil {
		return nil, false
	}
	e, ok := ctx.Value(elicitorContextKey{}).(Elicitor)
	return e, ok && e != nil
}
