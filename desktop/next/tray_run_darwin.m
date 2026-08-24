//go:build darwin

#import <Cocoa/Cocoa.h>
#import "_cgo_export.h"

void rxTrayStartOnMain(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        rxTrayStart();
    });
}
