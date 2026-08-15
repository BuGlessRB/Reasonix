//go:build darwin

#import <Cocoa/Cocoa.h>

// Hop to the main queue: AppKit reads and redraws the Dock tile there, and the
// caller is a Wails lifecycle hook that may not be it.
void setReasonixDockIcon(const void *data, int length) {
    if (data == NULL || length <= 0) {
        return;
    }
    NSData *bytes = [NSData dataWithBytes:data length:(NSUInteger)length];
    NSImage *icon = [[NSImage alloc] initWithData:bytes];
    if (icon == nil) {
        return;
    }
    dispatch_async(dispatch_get_main_queue(), ^{
        NSApplication.sharedApplication.applicationIconImage = icon;
    });
}
