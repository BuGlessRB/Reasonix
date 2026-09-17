// Package computer operates other applications on this machine for an agent,
// through a native helper that reads their accessibility trees, captures their
// windows, and puts input into them.
//
// The helper decides nothing; this package owns what the model is shown and
// what it may reach. An application is named by its bundle id, and some are
// never operated at all: this window, terminals, the keychain, password
// managers and System Settings, where an agent's input would reach past every
// other boundary the host keeps. Refs are the helper's, issued once per element
// and never reused. A click at a point is placed in the pixels of the
// application's latest screenshot and converted to screen points here, so the
// model never does display arithmetic.
//
// Nothing here moves the person's pointer or takes the foreground: a click is
// an accessibility action on the element under the point, shown by the helper's
// own cursor, and an element that has no such action answers computer.no_action
// rather than being clicked some other way. Escape pressed while that cursor is
// on screen stops the run between steps.
package computer
