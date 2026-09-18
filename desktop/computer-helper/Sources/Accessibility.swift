import AppKit
import ApplicationServices

// Refs name elements for the model. One is issued per element and never reused,
// so a ref read earlier cannot come to mean something else; an element that has
// gone answers computer.stale_ref.
final class RefTable {
    private var next = 0
    private var byHash: [CFHashCode: [(ref: String, element: AXUIElement)]] = [:]
    private var byRef: [String: (pid: pid_t, element: AXUIElement)] = [:]

    func ref(for element: AXUIElement, pid: pid_t) -> String {
        let hash = CFHash(element)
        if let found = byHash[hash]?.first(where: { CFEqual($0.element, element) }) {
            return found.ref
        }
        next += 1
        let ref = "a\(next)"
        byHash[hash, default: []].append((ref, element))
        byRef[ref] = (pid, element)
        return ref
    }

    func element(_ ref: String, pid: pid_t) throws -> AXUIElement {
        guard let entry = byRef[ref] else {
            throw Failure(code: "computer.unknown_ref", message: "\(ref) was never issued; refs come from a snapshot")
        }
        guard entry.pid == pid else {
            throw Failure(code: "computer.unknown_ref", message: "\(ref) belongs to another application")
        }
        var role: AnyObject?
        if AXUIElementCopyAttributeValue(entry.element, kAXRoleAttribute as CFString, &role) == .invalidUIElement {
            throw Failure(code: "computer.stale_ref", message: "\(ref) is no longer on screen; take a new snapshot")
        }
        return entry.element
    }
}

// Seen is one element's identity for a single walk: the accessibility API hands
// back a new reference for the same element, so equality is the API's, not the
// pointer's.
struct Seen: Hashable {
    let element: AXUIElement

    init(_ element: AXUIElement) { self.element = element }

    static func == (a: Seen, b: Seen) -> Bool { CFEqual(a.element, b.element) }

    func hash(into hasher: inout Hasher) { hasher.combine(CFHash(element)) }
}

enum Accessibility {
    static let maxNodes = 1500
    static let maxDepth = 40
    // Containers that say nothing when unnamed: their children stand in for them.
    static let transparent: Set<String> = ["AXGroup", "AXScrollArea", "AXSplitGroup", "AXLayoutArea", "AXUnknown"]

    static func app(_ pid: pid_t) -> AXUIElement {
        let element = AXUIElementCreateApplication(pid)
        AXUIElementSetMessagingTimeout(element, 1.5)
        return element
    }

    static func attr(_ element: AXUIElement, _ name: String) -> AnyObject? {
        var value: AnyObject?
        return AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success ? value : nil
    }

    static func string(_ element: AXUIElement, _ name: String) -> String {
        switch attr(element, name) {
        case let s as String: return s
        case let n as NSNumber: return n.stringValue
        default: return ""
        }
    }

    static func actions(_ element: AXUIElement) -> [String] {
        var names: CFArray?
        return AXUIElementCopyActionNames(element, &names) == .success ? (names as? [String] ?? []) : []
    }

    static func snapshot(pid: pid_t, refs: RefTable) throws -> JSON {
        try Permissions.requireAccessibility()
        guard let running = NSRunningApplication(processIdentifier: pid) else {
            throw Failure(code: "computer.no_app", message: "no application with pid \(pid) is running")
        }
        let root = app(pid)
        // Chromium-based applications build their tree only for a client that
        // asks; the switch is ignored by everything else.
        AXUIElementSetAttributeValue(root, "AXManualAccessibility" as CFString, kCFBooleanTrue)
        var windows = (attr(root, kAXWindowsAttribute) as? [AXUIElement]) ?? []
        if let focused = attr(root, kAXFocusedWindowAttribute), CFGetTypeID(focused) == AXUIElementGetTypeID() {
            let focusedWindow = focused as! AXUIElement
            windows.removeAll { CFEqual($0, focusedWindow) }
            windows.insert(focusedWindow, at: 0)
        }
        var lines: [String] = []
        var count = 0
        // An application's tree is not always one: a window of Calculator lists
        // the application among its children, and walking that lists the window
        // again. Every element is walked once per snapshot.
        var seen = Set<Seen>()
        func walk(_ element: AXUIElement, _ depth: Int) {
            if count >= maxNodes || depth > maxDepth { return }
            if !seen.insert(Seen(element)).inserted { return }
            count += 1
            let role = string(element, kAXRoleAttribute)
            var label = string(element, kAXTitleAttribute)
            if label.isEmpty { label = string(element, kAXDescriptionAttribute) }
            let value = string(element, kAXValueAttribute)
            if label.isEmpty && role == "AXStaticText" { label = value }
            let children = (attr(element, kAXChildrenAttribute) as? [AXUIElement]) ?? []
            if transparent.contains(role) && label.isEmpty {
                for child in children { walk(child, depth) }
                return
            }
            var line = String(repeating: "  ", count: depth) + "- " + roleName(role)
            if !label.isEmpty { line += " \(quote(label))" }
            line += " [\(refs.ref(for: element, pid: pid))]"
            if !value.isEmpty && value != label && role != "AXStaticText" { line += " value=\(quote(value))" }
            if let placeholder = attr(element, kAXPlaceholderValueAttribute) as? String, !placeholder.isEmpty, value.isEmpty {
                line += " placeholder=\(quote(placeholder))"
            }
            if (attr(element, kAXFocusedAttribute) as? Bool) == true { line += " focused" }
            if (attr(element, kAXEnabledAttribute) as? Bool) == false { line += " disabled" }
            if (attr(element, kAXSelectedAttribute) as? Bool) == true { line += " selected" }
            if actions(element).contains(kAXPressAction) { line += " pressable" }
            lines.append(line)
            for child in children { walk(child, depth + 1) }
        }
        for window in windows { walk(window, 0) }
        return [
            "app": running.localizedName ?? "",
            "bundle": running.bundleIdentifier ?? "",
            "lines": lines,
            "truncated": count >= maxNodes,
        ]
    }

    static func roleName(_ role: String) -> String {
        let bare = role.hasPrefix("AX") ? String(role.dropFirst(2)) : role
        return bare.isEmpty ? "element" : bare.prefix(1).lowercased() + bare.dropFirst()
    }

    static func quote(_ s: String) -> String {
        let clipped = s.count > 160 ? String(s.prefix(160)) + "…" : s
        let data = try? JSONSerialization.data(withJSONObject: [clipped])
        let json = data.flatMap { String(data: $0, encoding: .utf8) } ?? "[\"\"]"
        return String(json.dropFirst().dropLast())
    }

    static func center(_ element: AXUIElement) -> CGPoint? {
        guard let posValue = attr(element, kAXPositionAttribute), let sizeValue = attr(element, kAXSizeAttribute) else { return nil }
        var pos = CGPoint.zero, size = CGSize.zero
        guard AXValueGetValue(posValue as! AXValue, .cgPoint, &pos), AXValueGetValue(sizeValue as! AXValue, .cgSize, &size) else { return nil }
        return CGPoint(x: pos.x + size.width / 2, y: pos.y + size.height / 2)
    }

    static func perform(_ element: AXUIElement, _ action: String, what: String) throws {
        switch AXUIElementPerformAction(element, action as CFString) {
        case .success:
            return
        case .invalidUIElement:
            throw Failure(code: "computer.stale_ref", message: "\(what) is no longer on screen; take a new snapshot")
        case .actionUnsupported, .attributeUnsupported:
            throw Failure(code: "computer.no_action", message: "\(what) takes no \(action)")
        case let error:
            throw Failure(code: "computer.failed", message: "\(action) on \(what) failed (AXError \(error.rawValue))")
        }
    }

    static func press(pid: pid_t, ref: String, refs: RefTable, cursor: VirtualCursor) throws -> JSON {
        try Permissions.requireAccessibility()
        let element = try refs.element(ref, pid: pid)
        if let at = center(element) { cursor.move(to: at) }
        try perform(element, kAXPressAction, what: ref)
        return [:]
    }

    // click is a press at a point: the element there is asked for its action, so
    // the person's own pointer never moves. An element that has none cannot be
    // clicked this way, and says so rather than being clicked some other way.
    static func click(pid: pid_t, x: Double, y: Double, cursor: VirtualCursor) throws -> JSON {
        try Permissions.requireAccessibility()
        var hit: AXUIElement?
        guard AXUIElementCopyElementAtPosition(app(pid), Float(x), Float(y), &hit) == .success, let element = hit else {
            throw Failure(code: "computer.no_element", message: "nothing of this application is at (\(Int(x)), \(Int(y)))")
        }
        // Only an element of the application that was approved takes the click,
        // whatever the hit test resolves the point to.
        var owner: pid_t = 0
        guard AXUIElementGetPid(element, &owner) == .success, owner == pid else {
            throw Failure(code: "computer.no_element", message: "the element at (\(Int(x)), \(Int(y))) belongs to another application")
        }
        cursor.move(to: CGPoint(x: x, y: y))
        let role = string(element, kAXRoleAttribute)
        if actions(element).contains(kAXPressAction) {
            try perform(element, kAXPressAction, what: "the \(roleName(role)) at that point")
        } else if ["AXTextField", "AXTextArea", "AXComboBox", "AXSearchField"].contains(role) {
            AXUIElementSetAttributeValue(element, kAXFocusedAttribute as CFString, kCFBooleanTrue)
        } else {
            throw Failure(code: "computer.no_action", message: "the \(roleName(role)) at that point takes no accessibility action; it needs the real pointer")
        }
        return ["role": roleName(role)]
    }

    static func focus(pid: pid_t, ref: String, refs: RefTable, cursor: VirtualCursor) throws -> JSON {
        try Permissions.requireAccessibility()
        let element = try refs.element(ref, pid: pid)
        if let at = center(element) { cursor.move(to: at) }
        let result = AXUIElementSetAttributeValue(element, kAXFocusedAttribute as CFString, kCFBooleanTrue)
        if result != .success {
            throw Failure(code: "computer.no_action", message: "\(ref) cannot take focus (AXError \(result.rawValue))")
        }
        return [:]
    }

    static func setValue(pid: pid_t, ref: String, text: String, refs: RefTable, cursor: VirtualCursor) throws -> JSON {
        try Permissions.requireAccessibility()
        let element = try refs.element(ref, pid: pid)
        if let at = center(element) { cursor.move(to: at) }
        let result = AXUIElementSetAttributeValue(element, kAXValueAttribute as CFString, text as CFString)
        if result != .success {
            throw Failure(code: "computer.no_action", message: "\(ref) has no value that can be set (AXError \(result.rawValue))")
        }
        return ["value": string(element, kAXValueAttribute)]
    }
}
