#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

@interface Receipts : NSObject <NSApplicationDelegate, UNUserNotificationCenterDelegate>
@end
@implementation Receipts
- (void)applicationWillFinishLaunching:(NSNotification *)event {
    UNUserNotificationCenter.currentNotificationCenter.delegate = self;
}
- (void)applicationDidFinishLaunching:(NSNotification *)event {
    [UNUserNotificationCenter.currentNotificationCenter requestAuthorizationWithOptions:UNAuthorizationOptionAlert completionHandler:^(BOOL granted, NSError *error) {}];
}
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
    for (NSURL *url in urls) {
        if (![url.scheme isEqual:@"lansend-receipt"] || ![url.host isEqual:@"file"] || url.path.length < 2) continue;
        NSString *encoded = [[[url.path substringFromIndex:1] stringByReplacingOccurrencesOfString:@"-" withString:@"+"] stringByReplacingOccurrencesOfString:@"_" withString:@"/"];
        while (encoded.length % 4) encoded = [encoded stringByAppendingString:@"="];
        NSData *bytes = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
        NSString *name = bytes ? [[NSString alloc] initWithData:bytes encoding:NSUTF8StringEncoding] : nil;
        if (!name.length || ![name isEqual:name.lastPathComponent] || [name isEqual:@"."] || [name isEqual:@".."]) continue;
        NSString *path = [[NSHomeDirectory() stringByAppendingPathComponent:@"Downloads/LANSend"] stringByAppendingPathComponent:name];
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = @"LANSend 已收到文件";
        content.body = name;
        content.userInfo = @{@"path":path};
        UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:NSUUID.UUID.UUIDString content:content trigger:nil];
        UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
        [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert completionHandler:^(BOOL granted, NSError *error) {
            if (granted) [center addNotificationRequest:request withCompletionHandler:nil];
        }];
    }
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification withCompletionHandler:(void (^)(UNNotificationPresentationOptions))done {
    done(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)response withCompletionHandler:(void (^)(void))done {
    NSString *path = response.notification.request.content.userInfo[@"path"];
    if ([response.actionIdentifier isEqual:UNNotificationDefaultActionIdentifier] && path.length) {
        [NSWorkspace.sharedWorkspace activateFileViewerSelectingURLs:@[[NSURL fileURLWithPath:path]]];
    }
    done();
}
@end
int main(void) {
    @autoreleasepool {
        NSApplication *app = NSApplication.sharedApplication;
        Receipts * __attribute__((objc_precise_lifetime)) delegate = [Receipts new];
        app.delegate = delegate;
        [app run];
    }
    return 0;
}
