// Inspect geometry only. macOS can omit other applications' window titles.
#import <CoreGraphics/CoreGraphics.h>
#import <Foundation/Foundation.h>
int main(int argc, char **argv) {
    if(argc!=2) return 2;
    @autoreleasepool {
        NSArray *windows=CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID));
        for(NSDictionary *window in windows){
            if([window[(id)kCGWindowOwnerPID] intValue]!=atoi(argv[1])) continue;
            CGRect bounds;
            if(CGRectMakeWithDictionaryRepresentation((__bridge CFDictionaryRef)window[(id)kCGWindowBounds], &bounds)
               && bounds.size.width>=400 && bounds.size.height>=145) return 0;
        }
    }
    return 1;
}
