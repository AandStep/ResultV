// Copyright (C) 2026 ResultV
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This file is compiled WITHOUT -fobjc-arc (see dockquit_darwin.go). It uses
// manual retain/release because the swizzled applicationDockMenu: IMP must
// return an autoreleased NSMenu the way Apple's docs document — and ARC's
// objc_autoreleaseReturnValue optimisation through a method_setImplementation
// hook proved unreliable, deallocating the menu before Dock displayed it.

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#include "dockquit_darwin.h"

// Implemented in dockquit_darwin.go
extern void resultvDockQuitGoCallback(void);
extern void resultvDockShowGoCallback(void);
extern void resultvDockMenuRequestedGoCallback(void);

static BOOL gDockMenuInstalled = NO;
static id gDockQuitFinishLaunchingObserver = nil;
static id gDockMenuTarget = nil;

// DockMenuTarget hosts the "Показать ResultV" action. The "Завершить ResultV"
// item targets NSApp directly via the system terminate: selector — that path is
// already swizzled in terminate_darwin.m and goes through the graceful Wails
// BeforeClose → runShutdownTasks → os.Exit(0) sequence.
@interface ResultVDockMenuTarget : NSObject
- (void)resultvDockShowAction:(id)sender;
- (void)resultvDockQuitAction:(id)sender;
@end

@implementation ResultVDockMenuTarget
- (void)resultvDockShowAction:(__unused id)sender {
  resultvDockShowGoCallback();
}
- (void)resultvDockQuitAction:(__unused id)sender {
  // Route through Cocoa's standard terminate: so the same swizzled
  // applicationShouldTerminate: path runs (replyToApplicationShouldTerminate:
  // + graceful Wails BeforeClose).
  resultvDockQuitGoCallback();
  [NSApp terminate:nil];
}
@end

// Build a fresh autoreleased NSMenu every call. Apple's contract for
// applicationDockMenu: expects the returned menu to be autoreleased; Dock
// retains it for the duration of the popup and releases when dismissed.
static NSMenu *resultvBuildDockMenu(void) {
  if (gDockMenuTarget == nil) {
    gDockMenuTarget = [[ResultVDockMenuTarget alloc] init];
  }

  NSMenu *menu = [[[NSMenu alloc] initWithTitle:@""] autorelease];
  [menu setAutoenablesItems:NO];

  NSMenuItem *showItem = [[[NSMenuItem alloc]
      initWithTitle:@"Показать ResultV"
             action:@selector(resultvDockShowAction:)
      keyEquivalent:@""] autorelease];
  [showItem setTarget:gDockMenuTarget];
  [showItem setEnabled:YES];
  [menu addItem:showItem];

  [menu addItem:[NSMenuItem separatorItem]];

  NSMenuItem *quitItem = [[[NSMenuItem alloc]
      initWithTitle:@"Завершить ResultV"
             action:@selector(resultvDockQuitAction:)
      keyEquivalent:@""] autorelease];
  [quitItem setTarget:gDockMenuTarget];
  [quitItem setEnabled:YES];
  [menu addItem:quitItem];

  return menu;
}

static NSMenu *resultvApplicationDockMenu(id self, SEL _cmd, NSApplication *app) {
  (void)self;
  (void)_cmd;
  (void)app;
  resultvDockMenuRequestedGoCallback();
  return resultvBuildDockMenu();
}

// Force-install applicationDockMenu: on the delegate's class. class_addMethod
// silently no-ops if the selector already exists on the class hierarchy. Use
// class_replaceMethod to guarantee the IMP we want.
static void resultvForceInstallDockMenu(Class cls) {
  if (cls == NULL) {
    return;
  }
  if (!class_addMethod(cls, @selector(applicationDockMenu:),
                       (IMP)resultvApplicationDockMenu, "@@:@")) {
    class_replaceMethod(cls, @selector(applicationDockMenu:),
                        (IMP)resultvApplicationDockMenu, "@@:@");
  }
}

static void resultvInstallDockQuitMenuOnMainThread(void) {
  if (gDockMenuInstalled) {
    return;
  }
  id delegate = [NSApp delegate];
  if (delegate == nil) {
    return;
  }
  resultvForceInstallDockMenu([delegate class]);
  gDockMenuInstalled = YES;
}

// Late-bound installer: re-installs the swizzle as soon as macOS finishes
// launching NSApp. At this point Wails has definitely set its delegate and the
// LaunchServices process registration is complete.
static void resultvScheduleDockMenuOnFinishLaunching(void) {
  if (gDockQuitFinishLaunchingObserver != nil) {
    return;
  }
  void (^install)(NSNotification *) = ^(__unused NSNotification *note) {
    id delegate = [NSApp delegate];
    if (delegate != nil) {
      resultvForceInstallDockMenu([delegate class]);
      gDockMenuInstalled = YES;
    }
  };
  if ([NSApp isRunning]) {
    install(nil);
  }
  gDockQuitFinishLaunchingObserver = [[[NSNotificationCenter defaultCenter]
      addObserverForName:NSApplicationDidFinishLaunchingNotification
                  object:nil
                   queue:[NSOperationQueue mainQueue]
              usingBlock:install] retain];
}

int resultvInstallDockQuitMenuSync(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
    return gDockMenuInstalled ? 1 : 0;
  }
  __block BOOL ok = NO;
  dispatch_sync(dispatch_get_main_queue(), ^{
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
    ok = gDockMenuInstalled;
  });
  return ok ? 1 : 0;
}

void resultvInstallDockQuitMenu(void) {
  if ([NSThread isMainThread]) {
    resultvInstallDockQuitMenuOnMainThread();
    resultvScheduleDockMenuOnFinishLaunching();
  } else {
    dispatch_async(dispatch_get_main_queue(), ^{
      resultvInstallDockQuitMenuOnMainThread();
      resultvScheduleDockMenuOnFinishLaunching();
    });
  }
}
