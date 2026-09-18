import ApplicationServices
import Carbon.HIToolbox

// Keyboard input goes to one process, never to whatever is frontmost, so the
// person's own typing elsewhere is not interleaved with it.
enum Keyboard {
    static let named: [String: Int] = [
        "enter": kVK_Return, "return": kVK_Return, "tab": kVK_Tab, "escape": kVK_Escape, "space": kVK_Space,
        "backspace": kVK_Delete, "delete": kVK_ForwardDelete,
        "arrowleft": kVK_LeftArrow, "arrowright": kVK_RightArrow, "arrowup": kVK_UpArrow, "arrowdown": kVK_DownArrow,
        "home": kVK_Home, "end": kVK_End, "pageup": kVK_PageUp, "pagedown": kVK_PageDown,
        "a": kVK_ANSI_A, "c": kVK_ANSI_C, "v": kVK_ANSI_V, "x": kVK_ANSI_X, "z": kVK_ANSI_Z, "s": kVK_ANSI_S, "f": kVK_ANSI_F,
    ]
    static let modifiers: [String: CGEventFlags] = [
        "shift": .maskShift, "control": .maskControl, "ctrl": .maskControl,
        "alt": .maskAlternate, "option": .maskAlternate, "meta": .maskCommand, "cmd": .maskCommand,
    ]

    static func type(pid: pid_t, text: String) throws -> JSON {
        try Permissions.requireAccessibility()
        let source = CGEventSource(stateID: .privateState)
        let units = Array(text.utf16)
        var at = 0
        while at < units.count {
            let chunk = Array(units[at..<min(at + 16, units.count)])
            at += chunk.count
            for down in [true, false] {
                guard let event = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: down) else { continue }
                chunk.withUnsafeBufferPointer { event.keyboardSetUnicodeString(stringLength: chunk.count, unicodeString: $0.baseAddress) }
                event.postToPid(pid)
            }
        }
        return [:]
    }

    // hold keeps a key down, which is what a game or a scrubbing control reads
    // rather than a press.
    static func hold(pid: pid_t, chord: String, seconds: Double) throws -> JSON {
        let (code, flags) = try resolve(chord)
        let source = CGEventSource(stateID: .privateState)
        if let down = CGEvent(keyboardEventSource: source, virtualKey: CGKeyCode(code), keyDown: true) {
            down.flags = flags
            down.postToPid(pid)
        }
        Thread.sleep(forTimeInterval: min(max(seconds, 0), 30))
        if let up = CGEvent(keyboardEventSource: source, virtualKey: CGKeyCode(code), keyDown: false) {
            up.flags = flags
            up.postToPid(pid)
        }
        return [:]
    }

    // resolve reads a chord like "meta+s" into the key and the modifiers held
    // with it, and says what the vocabulary is when it cannot.
    static func resolve(_ chord: String) throws -> (Int, CGEventFlags) {
        try Permissions.requireAccessibility()
        let parts = chord.split(separator: "+").map { $0.trimmingCharacters(in: .whitespaces).lowercased() }
        guard let last = parts.last, let code = named[last] else {
            throw Failure(code: "computer.bad_step",
                          message: "\(chord) is not a key this can press. It presses named keys — \(named.keys.sorted().joined(separator: ", ")) — with the modifiers shift, control, alt and meta. Ordinary characters, digits included, go through the type action")
        }
        var flags = CGEventFlags()
        for part in parts.dropLast() {
            guard let flag = modifiers[part] else {
                throw Failure(code: "computer.bad_step", message: "\(part) is not a modifier; use shift, control, alt or meta")
            }
            flags.insert(flag)
        }
        return (code, flags)
    }

    static func press(pid: pid_t, chord: String, times: Int) throws -> JSON {
        let (code, flags) = try resolve(chord)
        let source = CGEventSource(stateID: .privateState)
        for _ in 0..<max(times, 1) {
            for down in [true, false] {
                guard let event = CGEvent(keyboardEventSource: source, virtualKey: CGKeyCode(code), keyDown: down) else { continue }
                event.flags = flags
                event.postToPid(pid)
            }
        }
        return [:]
    }
}
