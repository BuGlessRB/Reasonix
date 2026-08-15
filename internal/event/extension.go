// Extension UI surfaces. These structs mirror the Extension Protocol v2 UI
// payload DTOs field-for-field so any frontend can render them with native
// widgets; the protocol stays structured-only (no HTML/CSS/JS/URLs). All
// user-visible strings are credential-redacted by the host UI hub before the
// event is emitted.
package event

// Surface kinds carried by ExtensionSurfacePayload.Kind, mirroring the
// protocol's own. "request" is reserved: a blocking prompt currently routes
// through the ordinary AskRequest channel rather than arriving as a surface.
const (
	ExtensionSurfaceStatus       = "status"
	ExtensionSurfaceCard         = "card"
	ExtensionSurfaceForm         = "form"
	ExtensionSurfaceNotification = "notification"
	ExtensionSurfaceRequest      = "request"
)

// ExtensionSurfacePayload carries one sidecar's UI contribution for the
// ExtensionSurface / ExtensionStatus kinds. Exactly one sub-struct is set,
// selected by Kind.
type ExtensionSurfacePayload struct {
	PluginID     string
	SurfaceID    string
	SessionID    string
	Generation   uint64
	Kind         string // status | card | form | notification | panel
	Status       *ExtensionStatusView
	Card         *ExtensionCardView
	Form         *ExtensionFormView
	Notification *ExtensionNotificationView
	Panel        *ExtensionPanelView
}

// ExtensionPanelView is a standing surface for the frontend's side rail
// (mirrors UIPanelPayload). No Markdown: see the protocol DTO for why.
type ExtensionPanelView struct {
	Title    string
	Text     string
	Fields   []ExtensionKeyValue
	Progress *float64
	Actions  []ExtensionActionRef
}

// ExtensionStatusView is a one-line status contribution (mirrors the
// protocol's UIStatusPayload).
type ExtensionStatusView struct {
	Label    string
	Detail   string
	Severity string // info | warn | error
	Progress *float64
}

// ExtensionKeyValue is one labelled value row in a card (mirrors UIKeyValue).
type ExtensionKeyValue struct {
	Key   string
	Value string
}

// ExtensionActionRef renders a button invoking a declared extension action
// (mirrors UIActionRef).
type ExtensionActionRef struct {
	ActionID string
	Label    string
}

// ExtensionCardView is a rich read-only surface (mirrors UICardPayload).
type ExtensionCardView struct {
	Title    string
	Markdown string
	Text     string
	Fields   []ExtensionKeyValue
	Progress *float64
	Actions  []ExtensionActionRef
}

// ExtensionFormField is one input row of a form surface (mirrors UIFormField).
type ExtensionFormField struct {
	Key      string
	Label    string
	Kind     string // confirm | input | select | multiselect
	Options  []string
	Default  any
	Required bool
}

// ExtensionFormView is an editable surface; submissions return to the
// extension through the UI hub (mirrors UIFormPayload).
type ExtensionFormView struct {
	Title   string
	Message string
	Fields  []ExtensionFormField
}

// ExtensionNotificationView is a transient toast-style message (mirrors
// UINotificationPayload).
type ExtensionNotificationView struct {
	Title    string
	Body     string
	Severity string // info | warn | error
}
