import AppKit
import ScreenCaptureKit

enum Capture {
    // window captures an application's frontmost on-screen window by itself,
    // whatever covers it, at the display's pixel scale. bounds is where it sits
    // in global points, which is what a click at a point in the image maps to.
    static func window(pid: pid_t) throws -> JSON {
        guard #available(macOS 14.0, *) else {
            throw Failure(code: "computer.unsupported", message: "capturing one window needs macOS 14 or later")
        }
        return try capture(pid: pid)
    }

    @available(macOS 14.0, *)
    private static func capture(pid: pid_t) throws -> JSON {
        try Permissions.requireScreenRecording()
        let done = DispatchSemaphore(value: 0)
        var outcome: Result<JSON, Failure> = .failure(Failure(code: "computer.capture_failed", message: "no capture"))
        Task.detached {
            do {
                let content = try await SCShareableContent.excludingDesktopWindows(true, onScreenWindowsOnly: true)
                guard let target = content.windows
                    .filter({ $0.owningApplication?.processID == pid && $0.windowLayer == 0 && $0.frame.width > 1 })
                    .first else {
                    outcome = .failure(Failure(code: "computer.no_window", message: "the application has no window on screen"))
                    done.signal()
                    return
                }
                let filter = SCContentFilter(desktopIndependentWindow: target)
                let config = SCStreamConfiguration()
                config.width = Int(target.frame.width * CGFloat(filter.pointPixelScale))
                config.height = Int(target.frame.height * CGFloat(filter.pointPixelScale))
                config.showsCursor = false
                let image = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: config)
                let rep = NSBitmapImageRep(cgImage: image)
                guard let jpeg = rep.representation(using: .jpeg, properties: [.compressionFactor: 0.8]) else {
                    outcome = .failure(Failure(code: "computer.capture_failed", message: "the capture could not be encoded"))
                    done.signal()
                    return
                }
                outcome = .success([
                    "data": jpeg.base64EncodedString(),
                    "mime": "image/jpeg",
                    "width": image.width,
                    "height": image.height,
                    "window": target.windowID,
                    "bounds": ["x": target.frame.minX, "y": target.frame.minY, "width": target.frame.width, "height": target.frame.height],
                ])
            } catch {
                outcome = .failure(Failure(code: "computer.capture_failed", message: "\(error.localizedDescription)"))
            }
            done.signal()
        }
        // The capture completes off the main thread; waiting here keeps one reply
        // per request without parking the overlay's run loop for long.
        if done.wait(timeout: .now() + 10) == .timedOut {
            throw Failure(code: "computer.capture_failed", message: "the capture did not finish within 10s")
        }
        return try outcome.get()
    }
}
