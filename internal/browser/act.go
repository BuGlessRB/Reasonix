package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	objectGroup    = "reasonix"
	maxWait        = 30 * time.Second
	defaultWaitFor = 5 * time.Second
)

// Step is one input. X and Y are in the pixels of the tab's latest screenshot.
type Step struct {
	Action string   `json:"action"`
	Ref    string   `json:"ref,omitempty"`
	Text   string   `json:"text,omitempty"`
	Key    string   `json:"key,omitempty"`
	Values []string `json:"values,omitempty"`
	X      *float64 `json:"x,omitempty"`
	Y      *float64 `json:"y,omitempty"`
	DeltaY float64  `json:"delta_y,omitempty"`
	Ms     int      `json:"ms,omitempty"`
	Accept *bool    `json:"accept,omitempty"`
}

// ActResult is what a run of steps did before it finished or stopped.
type ActResult struct {
	Tab      TabInfo
	Done     int        // steps that completed
	Notes    []string   // one per completed step
	Logs     []LogEntry // errors and warnings the page reported meanwhile
	Opened   []string   // tabs the page opened meanwhile
	Dialog   *Dialog
	FailedAt int // index of the step that failed, or -1
}

// Act runs steps in order on a tab and stops at the first that fails. The
// returned error is that step's failure; the result says what came before it.
func (s *Session) Act(ctx context.Context, tabID string, steps []Step) (ActResult, error) {
	t, err := s.tab(tabID)
	if err != nil {
		return ActResult{FailedAt: 0}, err
	}
	if err := t.checkCurrentURL(); err != nil {
		return ActResult{FailedAt: 0}, err
	}
	_, logMark := t.logsAfter(1 << 62)
	t.mu.Lock()
	popupMark := len(t.popups)
	t.mu.Unlock()
	res := ActResult{FailedAt: -1}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		_ = t.call(releaseCtx, "Runtime.releaseObjectGroup", map[string]any{"objectGroup": objectGroup}, nil)
		cancel()
	}()
	var stepErr error
	for i, step := range steps {
		note, err := s.runStep(ctx, t, step)
		if err != nil {
			res.FailedAt, stepErr = i, err
			break
		}
		res.Done++
		res.Notes = append(res.Notes, note)
	}
	entries, _ := t.logsAfter(logMark)
	for _, e := range entries {
		if e.Level != "info" {
			res.Logs = append(res.Logs, e)
		}
	}
	t.mu.Lock()
	res.Opened = append(res.Opened, t.popups[popupMark:]...)
	t.mu.Unlock()
	active := s.activeTab()
	if active == nil {
		active = t
	}
	res.Tab = active.info(true)
	res.Dialog = active.currentDialog()
	return res, stepErr
}

func (s *Session) runStep(ctx context.Context, t *tab, step Step) (string, error) {
	action := strings.ToLower(strings.TrimSpace(step.Action))
	if d := t.currentDialog(); d != nil && action != "dialog" {
		return "", dialogFailure(d)
	}
	before := t.navigationCount()
	switch action {
	case "click", "double_click":
		x, y, err := s.point(ctx, t, step)
		if err != nil {
			return "", err
		}
		clicks := 1
		if action == "double_click" {
			clicks = 2
		}
		if err := t.click(ctx, x, y, clicks); err != nil {
			return "", err
		}
		t.settle(ctx, before)
		return fmt.Sprintf("%s %s", action, target(step)), nil
	case "hover":
		x, y, err := s.point(ctx, t, step)
		if err != nil {
			return "", err
		}
		if err := t.mouse(ctx, "mouseMoved", x, y, 0); err != nil {
			return "", err
		}
		return "hover " + target(step), nil
	case "type", "fill":
		if step.Ref != "" {
			if err := s.focus(ctx, t, step.Ref, action == "fill"); err != nil {
				return "", err
			}
		}
		if step.Text == "" && action == "fill" {
			if err := t.press(ctx, "Backspace"); err != nil {
				return "", err
			}
		} else if err := t.call(ctx, "Input.insertText", map[string]any{"text": step.Text}, nil); err != nil {
			return "", engineFailure(err)
		}
		return fmt.Sprintf("%s %d characters into %s", action, len([]rune(step.Text)), target(step)), nil
	case "press":
		if err := t.press(ctx, step.Key); err != nil {
			return "", err
		}
		t.settle(ctx, before)
		return "press " + step.Key, nil
	case "select":
		return s.selectOptions(ctx, t, step)
	case "scroll":
		x, y, err := s.scrollPoint(ctx, t, step)
		if err != nil {
			return "", err
		}
		if err := t.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": step.DeltaY}, nil); err != nil {
			return "", engineFailure(err)
		}
		return fmt.Sprintf("scroll %v px", step.DeltaY), nil
	case "wait":
		d := min(time.Duration(step.Ms)*time.Millisecond, maxWait)
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return fmt.Sprintf("wait %s", d), nil
	case "wait_for":
		return t.waitForText(ctx, step)
	case "dialog":
		return t.answerDialog(ctx, step)
	case "back":
		return t.back(ctx)
	}
	return "", fail(CodeBadStep, "unknown action %q", step.Action)
}

func target(step Step) string {
	if step.Ref != "" {
		return step.Ref
	}
	if step.X != nil && step.Y != nil {
		return fmt.Sprintf("(%v,%v)", *step.X, *step.Y)
	}
	return "the focused element"
}

// point is where an input for step lands, in CSS pixels. For a ref it is the
// middle of the element's visible part, and the element must be what a real
// pointer there would reach.
func (s *Session) point(ctx context.Context, t *tab, step Step) (float64, float64, error) {
	if step.Ref == "" {
		if step.X == nil || step.Y == nil {
			return 0, 0, fail(CodeBadStep, "a %s step needs a ref, or x and y from a screenshot", step.Action)
		}
		scale := t.screenshotScale()
		return *step.X * scale, *step.Y * scale, nil
	}
	node, err := s.node(ctx, t, step.Ref)
	if err != nil {
		return 0, 0, err
	}
	if err := t.call(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": node}, nil); err != nil {
		if isProtocolError(err) {
			return 0, 0, &Failure{Code: CodeNotVisible, Ref: step.Ref, Detail: fmt.Sprintf("%s has no box on the page (hidden or not rendered)", step.Ref)}
		}
		return 0, 0, engineFailure(err)
	}
	x, y, err := t.visibleCenter(ctx, node, step.Ref)
	if err != nil {
		return 0, 0, err
	}
	if err := s.checkHit(ctx, t, node, step.Ref, x, y); err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// node resolves a ref on t to a backend node that still exists.
func (s *Session) node(ctx context.Context, t *tab, ref string) (int64, error) {
	target, err := s.refs.resolve(ref)
	if err != nil {
		return 0, err
	}
	if target.tab != t.id {
		return 0, &Failure{Code: CodeUnknownRef, Ref: ref, Detail: fmt.Sprintf("%s is on tab %s; switch to it first", ref, target.tab)}
	}
	if err := t.call(ctx, "DOM.describeNode", map[string]any{"backendNodeId": target.node}, nil); err != nil {
		if isProtocolError(err) {
			return 0, &Failure{Code: CodeStaleRef, Ref: ref, Detail: fmt.Sprintf("%s is no longer on the page; take a new snapshot", ref)}
		}
		return 0, engineFailure(err)
	}
	return target.node, nil
}

func (t *tab) visibleCenter(ctx context.Context, node int64, ref string) (float64, float64, error) {
	var quads struct {
		Quads [][]float64 `json:"quads"`
	}
	if err := t.call(ctx, "DOM.getContentQuads", map[string]any{"backendNodeId": node}, &quads); err != nil && !isProtocolError(err) {
		return 0, 0, engineFailure(err)
	}
	var metrics struct {
		Viewport struct {
			Width  float64 `json:"clientWidth"`
			Height float64 `json:"clientHeight"`
		} `json:"cssLayoutViewport"`
	}
	if err := t.call(ctx, "Page.getLayoutMetrics", nil, &metrics); err != nil {
		return 0, 0, engineFailure(err)
	}
	for _, q := range quads.Quads {
		if len(q) != 8 {
			continue
		}
		minX, maxX := min(q[0], q[2], q[4], q[6]), max(q[0], q[2], q[4], q[6])
		minY, maxY := min(q[1], q[3], q[5], q[7]), max(q[1], q[3], q[5], q[7])
		minX, minY = max(minX, 0), max(minY, 0)
		maxX, maxY = min(maxX, metrics.Viewport.Width), min(maxY, metrics.Viewport.Height)
		if maxX-minX >= 1 && maxY-minY >= 1 {
			return (minX + maxX) / 2, (minY + maxY) / 2, nil
		}
	}
	return 0, 0, &Failure{Code: CodeNotVisible, Ref: ref, Detail: fmt.Sprintf("%s has no visible area in the viewport", ref)}
}

// checkHit refuses an input whose point would land on another element, such
// as a banner drawn over the target.
func (s *Session) checkHit(ctx context.Context, t *tab, node int64, ref string, x, y float64) error {
	var hit struct {
		BackendNodeID int64 `json:"backendNodeId"`
	}
	if err := t.call(ctx, "DOM.getNodeForLocation", map[string]any{"x": int(x), "y": int(y), "includeUserAgentShadowDOM": false, "ignorePointerEventsNone": true}, &hit); err != nil {
		if isProtocolError(err) {
			return nil
		}
		return engineFailure(err)
	}
	if hit.BackendNodeID == 0 || hit.BackendNodeID == node {
		return nil
	}
	targetObj, err := t.resolveObject(ctx, node)
	if err != nil {
		return err
	}
	hitObj, err := t.resolveObject(ctx, hit.BackendNodeID)
	if err != nil {
		return err
	}
	var within struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
	}
	if err := t.call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId":            targetObj,
		"functionDeclaration": "function(n){for(;n;n=n.parentNode||n.host){if(n===this)return true}return false}",
		"arguments":           []map[string]any{{"objectId": hitObj}},
		"returnByValue":       true,
	}, &within); err != nil {
		return engineFailure(err)
	}
	if within.Result.Value {
		return nil
	}
	cover := s.refs.refFor(t.id, hit.BackendNodeID)
	return &Failure{Code: CodeCovered, Ref: ref, Detail: fmt.Sprintf("%s is covered by %s %s at that point; deal with that element first", ref, t.describe(ctx, hit.BackendNodeID), cover)}
}

func (t *tab) resolveObject(ctx context.Context, node int64) (string, error) {
	var r struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := t.call(ctx, "DOM.resolveNode", map[string]any{"backendNodeId": node, "objectGroup": objectGroup}, &r); err != nil {
		if isProtocolError(err) {
			return "", fail(CodeStaleRef, "the element is no longer on the page")
		}
		return "", engineFailure(err)
	}
	return r.Object.ObjectID, nil
}

// describe names a DOM element by its tag and identifying attributes.
func (t *tab) describe(ctx context.Context, node int64) string {
	var d struct {
		Node struct {
			LocalName  string   `json:"localName"`
			Attributes []string `json:"attributes"`
		} `json:"node"`
	}
	if t.call(ctx, "DOM.describeNode", map[string]any{"backendNodeId": node}, &d) != nil {
		return "another element"
	}
	b := strings.Builder{}
	b.WriteString("<" + d.Node.LocalName)
	for i := 0; i+1 < len(d.Node.Attributes); i += 2 {
		switch d.Node.Attributes[i] {
		case "id", "class", "role", "aria-label":
			fmt.Fprintf(&b, " %s=%q", d.Node.Attributes[i], clip(d.Node.Attributes[i+1], 60))
		}
	}
	b.WriteString(">")
	return b.String()
}

func (t *tab) mouse(ctx context.Context, kind string, x, y float64, clicks int) error {
	params := map[string]any{"type": kind, "x": x, "y": y}
	if clicks > 0 {
		params["button"], params["clickCount"] = "left", clicks
	}
	return engineFailure(t.call(ctx, "Input.dispatchMouseEvent", params, nil))
}

func (t *tab) click(ctx context.Context, x, y float64, clicks int) error {
	if err := t.mouse(ctx, "mouseMoved", x, y, 0); err != nil {
		return err
	}
	for c := 1; c <= clicks; c++ {
		if err := t.mouse(ctx, "mousePressed", x, y, c); err != nil {
			return err
		}
		if err := t.mouse(ctx, "mouseReleased", x, y, c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) focus(ctx context.Context, t *tab, ref string, selectAll bool) error {
	node, err := s.node(ctx, t, ref)
	if err != nil {
		return err
	}
	if err := t.call(ctx, "DOM.focus", map[string]any{"backendNodeId": node}, nil); err != nil {
		if isProtocolError(err) {
			return &Failure{Code: CodeNotVisible, Ref: ref, Detail: fmt.Sprintf("%s cannot take focus", ref)}
		}
		return engineFailure(err)
	}
	if !selectAll {
		return nil
	}
	obj, err := t.resolveObject(ctx, node)
	if err != nil {
		return err
	}
	return engineFailure(t.call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId": obj,
		"functionDeclaration": `function(){
			if (typeof this.select === "function") { this.select(); return; }
			if (this.isContentEditable) {
				const r = document.createRange(); r.selectNodeContents(this);
				const s = getSelection(); s.removeAllRanges(); s.addRange(r);
			}
		}`,
	}, nil))
}

func (t *tab) press(ctx context.Context, chord string) error {
	def, modifiers, ok := parseKey(chord)
	if !ok {
		return fail(CodeBadStep, "%q is not a key this can press; use a name like Enter, Tab, ArrowDown, or a single character, with modifiers joined by +", chord)
	}
	down := map[string]any{
		"type": "keyDown", "key": def.key, "code": def.code, "modifiers": modifiers,
		"windowsVirtualKeyCode": def.vk, "nativeVirtualKeyCode": def.vk,
	}
	if def.text != "" {
		down["text"], down["unmodifiedText"] = def.text, def.text
	} else {
		down["type"] = "rawKeyDown"
	}
	if err := t.call(ctx, "Input.dispatchKeyEvent", down, nil); err != nil {
		return engineFailure(err)
	}
	return engineFailure(t.call(ctx, "Input.dispatchKeyEvent", map[string]any{
		"type": "keyUp", "key": def.key, "code": def.code, "modifiers": modifiers,
		"windowsVirtualKeyCode": def.vk, "nativeVirtualKeyCode": def.vk,
	}, nil))
}

func (s *Session) selectOptions(ctx context.Context, t *tab, step Step) (string, error) {
	node, err := s.node(ctx, t, step.Ref)
	if err != nil {
		return "", err
	}
	obj, err := t.resolveObject(ctx, node)
	if err != nil {
		return "", err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	if err := t.call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId": obj,
		"functionDeclaration": `function(values){
			if (!(this instanceof HTMLSelectElement)) return -1;
			const want = new Set(values); let hit = 0;
			for (const o of this.options) {
				const on = want.has(o.value) || want.has(o.label);
				if (on || !this.multiple) o.selected = on;
				if (on) hit++;
			}
			if (hit) { this.dispatchEvent(new Event("input", {bubbles: true})); this.dispatchEvent(new Event("change", {bubbles: true})); }
			return hit;
		}`,
		"arguments":     []map[string]any{{"value": step.Values}},
		"returnByValue": true,
	}, &r); err != nil {
		return "", engineFailure(err)
	}
	var hit int
	_ = json.Unmarshal(r.Result.Value, &hit)
	switch {
	case hit < 0:
		return "", &Failure{Code: CodeNotSelectable, Ref: step.Ref, Detail: fmt.Sprintf("%s is not a <select>; click it and choose from what opens instead", step.Ref)}
	case hit == 0:
		return "", &Failure{Code: CodeNoSuchOption, Ref: step.Ref, Detail: fmt.Sprintf("%s has no option with value or label %q", step.Ref, step.Values)}
	}
	return fmt.Sprintf("select %d option(s) in %s", hit, step.Ref), nil
}

func (s *Session) scrollPoint(ctx context.Context, t *tab, step Step) (float64, float64, error) {
	if step.Ref != "" || (step.X != nil && step.Y != nil) {
		return s.point(ctx, t, step)
	}
	var metrics struct {
		Viewport struct {
			Width  float64 `json:"clientWidth"`
			Height float64 `json:"clientHeight"`
		} `json:"cssLayoutViewport"`
	}
	if err := t.call(ctx, "Page.getLayoutMetrics", nil, &metrics); err != nil {
		return 0, 0, engineFailure(err)
	}
	return metrics.Viewport.Width / 2, metrics.Viewport.Height / 2, nil
}

func (t *tab) waitForText(ctx context.Context, step Step) (string, error) {
	if step.Text == "" {
		return "", fail(CodeBadStep, "a wait_for step needs text to wait for")
	}
	limit := defaultWaitFor
	if step.Ms > 0 {
		limit = min(time.Duration(step.Ms)*time.Millisecond, maxWait)
	}
	quoted, _ := json.Marshal(step.Text)
	expr := fmt.Sprintf("!!document.body && document.body.innerText.includes(%s)", quoted)
	deadline := time.Now().Add(limit)
	for {
		var r struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if err := t.call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true}, &r); err != nil && !isProtocolError(err) {
			return "", engineFailure(err)
		}
		if r.Result.Value {
			return fmt.Sprintf("found %q", step.Text), nil
		}
		if time.Now().After(deadline) {
			return "", fail(CodeWaitTimeout, "%q did not appear within %s", step.Text, limit)
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func (t *tab) answerDialog(ctx context.Context, step Step) (string, error) {
	d := t.currentDialog()
	if d == nil {
		return "", fail(CodeNoDialog, "no dialog is open")
	}
	accept := step.Accept == nil || *step.Accept
	params := map[string]any{"accept": accept}
	if step.Text != "" {
		params["promptText"] = step.Text
	}
	if err := t.call(ctx, "Page.handleJavaScriptDialog", params, nil); err != nil {
		return "", engineFailure(err)
	}
	verb := "dismissed"
	if accept {
		verb = "accepted"
	}
	return fmt.Sprintf("%s the %s dialog", verb, d.Type), nil
}

func (t *tab) back(ctx context.Context) (string, error) {
	var h struct {
		CurrentIndex int `json:"currentIndex"`
		Entries      []struct {
			ID int `json:"id"`
		} `json:"entries"`
	}
	if err := t.call(ctx, "Page.getNavigationHistory", nil, &h); err != nil {
		return "", engineFailure(err)
	}
	if h.CurrentIndex <= 0 || h.CurrentIndex >= len(h.Entries) {
		return "", fail(CodeNavigationFailed, "there is no earlier page in this tab")
	}
	before := t.navigationCount()
	if err := t.call(ctx, "Page.navigateToHistoryEntry", map[string]any{"entryId": h.Entries[h.CurrentIndex-1].ID}, nil); err != nil {
		return "", engineFailure(err)
	}
	t.settle(ctx, before)
	url, _ := t.location()
	return "back to " + url, nil
}
