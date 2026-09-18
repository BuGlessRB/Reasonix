import AppKit

// The application the live tests operate. Every input it receives is appended to
// the log named by argv[1], so a test can tell what arrived without looking.
let logPath = CommandLine.arguments[1]
func log(_ s: String) {
    let line = s + "\n"
    if let h = FileHandle(forWritingAtPath: logPath) { h.seekToEndOfFile(); h.write(line.data(using: .utf8)!); h.closeFile() }
    else { FileManager.default.createFile(atPath: logPath, contents: line.data(using: .utf8)) }
}

final class Field: NSTextField {
    override func keyDown(with event: NSEvent) { log("keyDown \(event.characters ?? "")"); super.keyDown(with: event) }
}
final class ClickView: NSView {
    override func mouseDown(with event: NSEvent) { log("mouseDown \(Int(event.locationInWindow.x)),\(Int(event.locationInWindow.y))") }
    override func draw(_ dirtyRect: NSRect) { NSColor.systemRed.setFill(); dirtyRect.fill() }
}
// A view with a menu of its own, so a context-menu action has something to open,
// and one that says when it is scrolled into view.
final class MenuView: NSView {
    override func menu(for event: NSEvent) -> NSMenu? {
        log("menu opened")
        let menu = NSMenu()
        menu.addItem(NSMenuItem(title: "Probe menu item", action: nil, keyEquivalent: ""))
        return menu
    }
}
final class DeepLabel: NSTextField {
    override func scrollToVisible(_ rect: NSRect) -> Bool {
        log("scrolled into view")
        return super.scrollToVisible(rect)
    }
}

final class Delegate: NSObject, NSApplicationDelegate, NSTextFieldDelegate {
    var window: NSWindow!
    func applicationDidFinishLaunching(_ n: Notification) {
        window = NSWindow(contentRect: NSRect(x: 80, y: 80, width: 420, height: 220), styleMask: [.titled], backing: .buffered, defer: false)
        window.title = "Reasonix Probe Target"
        let field = Field(frame: NSRect(x: 20, y: 150, width: 260, height: 24))
        field.identifier = NSUserInterfaceItemIdentifier("probe-field")
        field.setAccessibilityLabel("Probe field")
        field.delegate = self
        let button = NSButton(title: "Probe button", target: self, action: #selector(pressed))
        button.frame = NSRect(x: 290, y: 148, width: 110, height: 28)
        let click = ClickView(frame: NSRect(x: 20, y: 20, width: 160, height: 100))
        let menuView = MenuView(frame: NSRect(x: 200, y: 20, width: 100, height: 60))
        menuView.setAccessibilityLabel("Probe menu area")
        menuView.setAccessibilityRole(.button)
        menuView.setAccessibilityElement(true)
        let scroll = NSScrollView(frame: NSRect(x: 310, y: 20, width: 90, height: 100))
        let deep = DeepLabel(labelWithString: "deep row")
        deep.setAccessibilityLabel("Deep row")
        deep.frame = NSRect(x: 0, y: 0, width: 80, height: 20)
        let doc = NSView(frame: NSRect(x: 0, y: 0, width: 80, height: 600))
        doc.addSubview(deep)
        scroll.documentView = doc
        scroll.hasVerticalScroller = true
        window.contentView?.addSubview(field)
        window.contentView?.addSubview(button)
        window.contentView?.addSubview(click)
        window.contentView?.addSubview(menuView)
        window.contentView?.addSubview(scroll)
        window.orderFrontRegardless()
        log("ready pid=\(ProcessInfo.processInfo.processIdentifier)")
    }
    @objc func pressed() { log("button pressed") }
    func controlTextDidChange(_ obj: Notification) { log("text \((obj.object as! NSTextField).stringValue)") }
}
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let d = Delegate()
app.delegate = d
app.run()
