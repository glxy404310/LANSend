#import <Cocoa/Cocoa.h>

@interface Desktop:NSObject <NSApplicationDelegate,NSWindowDelegate>
@property NSWindow *window;
@property NSTextField *peer,*status,*info,*address,*code;
@property NSButton *sendButton,*pairButton;
@property NSProgressIndicator *progress;
@property NSView *transfer,*settings,*devices;
@property NSPopUpButton *picker;
@property NSTextField *deviceHint;
@property BOOL scanning;
@property NSArray *lastDevices;
@property NSTimer *scanTimer;
@property BOOL busy;
@property NSString *engine;
@end
@implementation Desktop
- (NSTextField*)label:(NSString*)value frame:(NSRect)rect size:(CGFloat)size bold:(BOOL)bold view:(NSView*)parent {
    NSTextField *f=[NSTextField labelWithString:value]; f.frame=rect;
    f.font=[NSFont systemFontOfSize:size weight:bold?NSFontWeightSemibold:NSFontWeightRegular];
    f.lineBreakMode=NSLineBreakByTruncatingMiddle; [parent addSubview:f];return f;
}
- (NSButton*)button:(NSString*)title at:(NSRect)rect action:(SEL)action view:(NSView*)parent {
    NSButton *b=[NSButton buttonWithTitle:title target:self action:action];b.frame=rect;[parent addSubview:b];return b;
}
- (void)refreshPeer {
    NSString *path=[NSHomeDirectory() stringByAppendingPathComponent:@"Library/Application Support/LANSend/peer.json"];
    NSData *data=[NSData dataWithContentsOfFile:path];NSDictionary *c=data?[NSJSONSerialization JSONObjectWithData:data options:0 error:nil]:nil;
    self.peer.stringValue=c[@"Address"]?[NSString stringWithFormat:@"已配对   ·   %@",c[@"Address"]]:@"尚未配对";
    if([c[@"RemoteAddress"] length])self.peer.stringValue=[self.peer.stringValue stringByAppendingFormat:@"   /   %@",c[@"RemoteAddress"]];
}
- (void)run:(NSString*)program args:(NSArray*)args input:(NSString*)input done:(void(^)(BOOL,NSString*))done {
    self.busy=YES;self.sendButton.enabled=NO;self.pairButton.enabled=NO;
    dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0),^{
        NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:program];task.arguments=args;
        NSPipe *output=[NSPipe pipe];task.standardOutput=output;task.standardError=output;
        NSPipe *feed=[NSPipe pipe];task.standardInput=feed;
        NSError *e=nil; BOOL ok=[task launchAndReturnError:&e];NSMutableData *tail=[NSMutableData data];
        if(ok){
            if(input)[feed.fileHandleForWriting writeData:[[input stringByAppendingString:@"\n"] dataUsingEncoding:NSUTF8StringEncoding]];
            [feed.fileHandleForWriting closeFile];
            while(YES){NSData *part=output.fileHandleForReading.availableData;if(!part.length)break;
                [tail appendData:part];if(tail.length>131072)[tail replaceBytesInRange:NSMakeRange(0,tail.length-65536) withBytes:NULL length:0];
                if([args.firstObject isEqual:@"send"]){NSString *s=[[NSString alloc]initWithData:part encoding:NSUTF8StringEncoding];
                    NSString *line=[[s stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] componentsSeparatedByCharactersInSet:NSCharacterSet.newlineCharacterSet].lastObject;
                    if(line.length)dispatch_async(dispatch_get_main_queue(),^{self.status.stringValue=line;});
                }
            }
            [task waitUntilExit];ok=task.terminationStatus==0;
        }
        NSString *message=e.localizedDescription?:([[NSString alloc]initWithData:tail encoding:NSUTF8StringEncoding]?:@"");
        dispatch_async(dispatch_get_main_queue(),^{self.busy=NO;self.sendButton.enabled=YES;self.pairButton.enabled=YES;[self.progress stopAnimation:nil];done(ok,[message stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]);});
    });
}
- (void)error:(NSString*)msg {NSAlert *a=[NSAlert new];a.messageText=@"操作未完成";a.informativeText=msg;[a beginSheetModalForWindow:self.window completionHandler:nil];}
- (void)send:(id)sender {
    NSOpenPanel *p=NSOpenPanel.openPanel;p.allowsMultipleSelection=YES;p.canChooseDirectories=NO;p.prompt=@"发送";
    [p beginSheetModalForWindow:self.window completionHandler:^(NSModalResponse result){
        if(result!=NSModalResponseOK)return;
        NSMutableArray *args=[NSMutableArray arrayWithObject:@"send"];for(NSURL *u in p.URLs)[args addObject:u.path];
        self.status.stringValue=@"正在发送…";[self.progress startAnimation:nil];
        [self run:self.engine args:args input:nil done:^(BOOL ok,NSString *msg){
            self.status.stringValue=ok?[NSString stringWithFormat:@"已发送 %lu 个文件",(unsigned long)(args.count-1)]:@"发送未完成";if(!ok)[self error:[msg componentsSeparatedByString:@"\n"].lastObject];
        }];
    }];
}
- (void)pair:(id)sender {
    if(!self.address.stringValue.length || self.code.stringValue.length!=12){[self error:@"请输入对方的 IP 地址和 12 位校验码。"];return;}
    [self run:self.engine args:@[@"pair",self.address.stringValue] input:self.code.stringValue done:^(BOOL ok,NSString *msg){
        if(ok){self.code.stringValue=@"";[self refreshPeer];self.status.stringValue=@"已保存配对";}else [self error:msg];
    }];
}
- (void)remote:(id)sender {
 if(self.busy)return;
 [self run:self.engine args:@[@"remote-login"] input:nil done:^(BOOL ok,NSString *msg){
 if(!ok){[self error:msg];return;}
 NSAlert *a=[NSAlert new];a.messageText=@"远程连接";a.informativeText=msg;[a beginSheetModalForWindow:self.window completionHandler:nil];
 }];
}
- (void)inbox:(id)sender {NSURL *u=[NSURL fileURLWithPath:[NSHomeDirectory() stringByAppendingPathComponent:@"Downloads/LANSend"] isDirectory:YES];[NSWorkspace.sharedWorkspace openURL:u];}
- (void)scan {
 if(self.devices.hidden || self.busy || self.scanning || !self.engine)return;
 self.scanning=YES;
 dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0),^{
 NSTask *task=[NSTask new];task.executableURL=[NSURL fileURLWithPath:self.engine];task.arguments=@[@"devices"];
 NSPipe *pipe=[NSPipe pipe];task.standardOutput=pipe;task.standardError=pipe;NSError *err=nil;
 NSArray *items=nil;NSString *failure=nil;
 if([task launchAndReturnError:&err]){NSData *data=[pipe.fileHandleForReading readDataToEndOfFile];[task waitUntilExit];
 if(task.terminationStatus==0)items=[NSJSONSerialization JSONObjectWithData:data options:0 error:nil];else failure=[[NSString alloc]initWithData:data encoding:NSUTF8StringEncoding];
 }else failure=err.localizedDescription;
 dispatch_async(dispatch_get_main_queue(),^{self.scanning=NO;

 if(![self.lastDevices isEqual:items]){self.lastDevices=items;NSString *prev=self.picker.selectedItem.representedObject;
 [self.picker removeAllItems];for(NSDictionary *p in items){NSMenuItem *item=[[NSMenuItem alloc]initWithTitle:p[@"Name"] action:nil keyEquivalent:@""];item.representedObject=p[@"ID"];[self.picker.menu addItem:item];}
 NSInteger idx=prev?[self.picker indexOfItemWithRepresentedObject:prev]:-1;if(self.picker.numberOfItems)[self.picker selectItemAtIndex:idx<0&&items.count==1?0:idx];}
 self.deviceHint.stringValue=failure?[failure stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]:(items.count?@"选择要双向配对的电脑":@"等待另一台 LANSend 登录同一账号…");});
 });
}
- (void)connectDevice:(id)sender {
 NSString *device=self.picker.selectedItem.representedObject;if(self.busy||!device)return;
 [self run:self.engine args:@[@"connect-device",device] input:nil done:^(BOOL ok,NSString *msg){
 if(!ok){[self error:msg];return;}[self refreshPeer];NSAlert *a=[NSAlert new];a.messageText=@"配对完成";a.informativeText=@"两台电脑现在可以互传文件。";[a beginSheetModalForWindow:self.window completionHandler:nil];}];
}
- (void)tab:(NSSegmentedControl*)c {self.transfer.hidden=c.selectedSegment!=0;self.settings.hidden=c.selectedSegment!=1;self.devices.hidden=c.selectedSegment!=2;[self scan];}
- (BOOL)windowShouldClose:(NSWindow*)sender {if(self.busy){NSBeep();return NO;}return YES;}
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication*)sender{return YES;}
- (void)applicationDidFinishLaunching:(NSNotification*)note {
    NSMenu *main=[NSMenu new];NSMenuItem *appItem=[NSMenuItem new];[main addItem:appItem];NSMenu *appMenu=[NSMenu new];appItem.submenu=appMenu;
    [appMenu addItemWithTitle:@"退出 LANSend" action:@selector(terminate:) keyEquivalent:@"q"];
    NSMenuItem *edit=[NSMenuItem new];[main addItem:edit];edit.submenu=[[NSMenu alloc]initWithTitle:@"编辑"];
    for(NSArray *a in @[@[@"剪切",@"cut:",@"x"],@[@"复制",@"copy:",@"c"],@[@"粘贴",@"paste:",@"v"],@[@"全选",@"selectAll:",@"a"]])[edit.submenu addItemWithTitle:a[0] action:NSSelectorFromString(a[1]) keyEquivalent:a[2]];
    NSApp.mainMenu=main;
    self.window=[[NSWindow alloc]initWithContentRect:NSMakeRect(0,0,580,410) styleMask:NSWindowStyleMaskTitled|NSWindowStyleMaskClosable|NSWindowStyleMaskMiniaturizable backing:NSBackingStoreBuffered defer:NO];
    self.window.title=@"LANSend";self.window.delegate=self;self.window.releasedWhenClosed=NO;
    NSView *v=self.window.contentView;NSView *header=[[NSView alloc]initWithFrame:NSMakeRect(0,304,580,106)];header.wantsLayer=YES;header.layer.backgroundColor=[NSColor colorWithSRGBRed:.075 green:.208 blue:.239 alpha:1].CGColor;[v addSubview:header];
    [self label:@"LANSend" frame:NSMakeRect(28,49,510,39) size:30 bold:YES view:header].textColor=NSColor.whiteColor;
    self.peer=[self label:@"" frame:NSMakeRect(30,21,520,24) size:14 bold:NO view:header];self.peer.textColor=NSColor.whiteColor;[self refreshPeer];
    NSSegmentedControl *tabs=[NSSegmentedControl segmentedControlWithLabels:@[@"文件传输",@"设备配对",@"我的电脑"] trackingMode:NSSegmentSwitchTrackingSelectOne target:self action:@selector(tab:)];tabs.frame=NSMakeRect(26,258,320,30);tabs.selectedSegment=0;[v addSubview:tabs];
    self.transfer=[[NSView alloc]initWithFrame:NSMakeRect(24,16,530,232)];[v addSubview:self.transfer];self.settings=[[NSView alloc]initWithFrame:self.transfer.frame];[v addSubview:self.settings];self.settings.hidden=YES;
    [self label:@"发送到 Windows" frame:NSMakeRect(4,175,470,32) size:22 bold:YES view:self.transfer];
    self.sendButton=[self button:@"选择文件并发送…" at:NSMakeRect(0,115,170,38) action:@selector(send:) view:self.transfer];self.sendButton.bezelColor=[NSColor colorWithSRGBRed:.1 green:.46 blue:.41 alpha:1];
    [self button:@"打开收件箱" at:NSMakeRect(180,115,124,38) action:@selector(inbox:) view:self.transfer];
    self.status=[self label:@"正在启动接收服务…" frame:NSMakeRect(4,32,520,25) size:14 bold:NO view:self.transfer];
    self.progress=[[NSProgressIndicator alloc]initWithFrame:NSMakeRect(5,17,515,8)];self.progress.indeterminate=YES;self.progress.displayedWhenStopped=NO;[self.transfer addSubview:self.progress];
    self.info=[self label:@"" frame:NSMakeRect(4,151,520,74) size:14 bold:NO view:self.settings];self.info.maximumNumberOfLines=3;self.info.selectable=YES;
    self.address=[NSTextField new];self.address.frame=NSMakeRect(4,111,355,27);self.address.placeholderString=@"对方的局域网 IP 或 Tailscale 地址";[self.settings addSubview:self.address];
    self.code=[NSTextField new];self.code.frame=NSMakeRect(4,73,355,27);self.code.placeholderString=@"对方的 12 位校验码";[self.settings addSubview:self.code];
    [self button:@"远程连接…" at:NSMakeRect(168,24,126,36) action:@selector(remote:) view:self.settings];
    self.pairButton=[self button:@"验证并保存配对" at:NSMakeRect(0,24,164,36) action:@selector(pair:) view:self.settings];
    self.devices=[[NSView alloc]initWithFrame:self.transfer.frame];[v addSubview:self.devices];self.devices.hidden=YES;
 self.deviceHint=[self label:@"正在查找同一账号的电脑…" frame:NSMakeRect(4,174,518,28) size:14 bold:NO view:self.devices];
 self.picker=[[NSPopUpButton alloc]initWithFrame:NSMakeRect(4,120,390,34) pullsDown:NO];[self.devices addSubview:self.picker];
 [self button:@"配对所选电脑" at:NSMakeRect(0,60,165,38) action:@selector(connectDevice:) view:self.devices];
 [self button:@"远程连接…" at:NSMakeRect(168,60,125,38) action:@selector(remote:) view:self.devices];
 self.scanTimer=[NSTimer scheduledTimerWithTimeInterval:5 repeats:YES block:^(NSTimer *timer){[self refreshPeer];[self scan];}];
 [self.window center];[self.window makeKeyAndOrderFront:nil];[NSApp activateIgnoringOtherApps:YES];
    NSString *res=NSBundle.mainBundle.resourcePath;self.engine=[res stringByAppendingPathComponent:@"engine"];
    [self run:@"/bin/bash" args:@[[res stringByAppendingPathComponent:@"Setup.sh"]] input:nil done:^(BOOL ok,NSString *msg){
        if(ok){self.info.stringValue=msg;self.status.stringValue=@"接收服务已就绪";}else{self.status.stringValue=@"接收服务启动失败";[self error:msg];}
    }];
}
@end
int main(void){@autoreleasepool{NSApplication *app=NSApplication.sharedApplication;Desktop * __attribute__((objc_precise_lifetime)) delegate=[Desktop new];app.delegate=delegate;[app run];}return 0;}
