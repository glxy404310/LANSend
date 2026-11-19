// Run with CFFIXED_USER_HOME pointing to an empty test directory.
#define main LANSendApplicationMain
#import "../main.m"
#undef main
#define CHECK(condition) do { if(!(condition)){ NSLog(@"FAILED at line %d: %s",__LINE__,#condition); return 1; } } while(0)

int main(void) {
    @autoreleasepool {
        const char *temporary=getenv("CFFIXED_USER_HOME");
        CHECK(temporary && [NSHomeDirectory().stringByResolvingSymlinksInPath isEqual:[@(temporary) stringByResolvingSymlinksInPath]]);
        [NSApplication sharedApplication];
        Desktop *app=[Desktop new];
        NSString *dir=app.silentSettingPath.stringByDeletingLastPathComponent;
        CHECK([NSFileManager.defaultManager createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:nil]);
        CHECK(!app.silentReceipts);
        [app showReceived:@"甲 & 1.txt"];
        NSPanel *panel=app.receipt;
        [app showReceived:@"甲 & 1 (1).txt"];
        CHECK(panel.visible && app.receipt==panel && app.files.numberOfItems==2);
        CHECK([app.files.selectedItem.title isEqual:@"甲 & 1.txt"]);
        NSSegmentedControl *mode=[NSSegmentedControl segmentedControlWithLabels:@[@"前台弹窗",@"静默接收"] trackingMode:NSSegmentSwitchTrackingSelectOne target:nil action:nil];
        mode.selectedSegment=1;
        [app changeMode:mode];
        CHECK(!panel.visible);
        Desktop *next=[Desktop new];
        CHECK(next.silentReceipts);
        [next showReceived:@"静默.txt"];
        CHECK(!next.receipt);
        mode.selectedSegment=0;
        [next changeMode:mode];
        CHECK(!app.silentReceipts);
        [app showReceived:@"新批次.txt"];
        CHECK(panel.visible && app.files.numberOfItems==1 && [app.files.selectedItem.title isEqual:@"新批次.txt"]);
    }
    return 0;
}
